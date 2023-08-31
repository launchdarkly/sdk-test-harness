package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/data"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldattr"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/require"
)

func doServerSideFeatureEventTests(t *ldtest.T) {
	valueFactories := data.MakeValueFactoriesBySDKValueType(2)
	flagValues, defaultValues := valueFactories[0], valueFactories[1]
	contexts := data.NewContextFactory("doServerSideFeatureEventTests")
	expectedReason := ldreason.NewEvalReasonFallthrough()
	untrackedFlags := data.NewFlagFactory(
		"untracked-flag",
		flagValues,
		data.FlagShouldProduceThisEvalReason(expectedReason),
	)
	trackedFlags := data.NewFlagFactory(
		"tracked-flag",
		flagValues,
		data.FlagShouldProduceThisEvalReason(expectedReason),
		data.FlagShouldHaveFullEventTracking,
	)
	malformedFlag := ldbuilders.NewFlagBuilder("bad-flag").Version(1).
		On(false).OffVariation(-1).TrackEvents(true).Build()
	zeroSamplingRatioFlag := ldbuilders.NewFlagBuilder("zero-sampling-ratio").Version(1).
		On(true).Variations(ldvalue.Bool(true), ldvalue.Bool(false)).SamplingRatio(0).Build()

	dataBuilder := mockld.NewServerSDKDataBuilder()
	for _, valueType := range getValueTypesToTest(t) {
		dataBuilder.Flag(untrackedFlags.MakeFlagForValueType(valueType))
		dataBuilder.Flag(trackedFlags.MakeFlagForValueType(valueType))
	}
	dataBuilder.Flag(malformedFlag, zeroSamplingRatioFlag)

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSink(t)

	client := NewSDKClient(t, dataSource, events)

	t.Run("only index + summary event for untracked flag", func(t *ldtest.T) {
		for _, withReason := range []bool{false, true} {
			t.Run(h.IfElse(withReason, "with reasons", "without reasons"), func(t *ldtest.T) {
				for _, valueType := range getValueTypesToTest(t) {
					t.Run(testDescFromType(valueType), func(t *ldtest.T) {
						flag := untrackedFlags.ReuseFlagForValueType(valueType)
						context := contexts.NextUniqueContext()

						resp := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
							FlagKey:      flag.Key,
							Context:      o.Some(context),
							ValueType:    valueType,
							DefaultValue: defaultValues(valueType),
							Detail:       withReason,
						})

						// If the evaluation didn't return the expected value, then the rest of the test is moot
						if !m.In(t).Assert(flag.Variations[0], m.JSONEqual(resp.Value)) {
							require.Fail(t, "evaluation unexpectedly returned wrong value")
						}
						if withReason {
							m.In(t).Assert(resp.Reason, m.JSONEqual(expectedReason))
						} else {
							m.In(t).Assert(resp.Reason, m.JSONEqual(ldreason.EvaluationReason{}))
						}

						client.FlushEvents(t)
						payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
						m.In(t).Assert(payload, m.ItemsInAnyOrder(
							IsIndexEventForContext(context),
							IsSummaryEvent(),
						))
					})
				}
			})
		}
	})

	doFeatureEventTest := func(t *ldtest.T, contexts *data.ContextFactory, withReason, isBadFlag bool) {
		for _, valueType := range getValueTypesToTest(t) {
			t.Run(testDescFromType(valueType), func(t *ldtest.T) {
				flag := trackedFlags.ReuseFlagForValueType(valueType)
				expectedValue := flagValues(valueType)
				expectedVariation := o.Some(0)
				if isBadFlag {
					flag = malformedFlag
					expectedValue = defaultValues(valueType)
					expectedVariation = o.None[int]()
				}
				context := contexts.NextUniqueContext()
				resp := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
					FlagKey:      flag.Key,
					Context:      o.Some(context),
					ValueType:    valueType,
					DefaultValue: defaultValues(valueType),
					Detail:       withReason,
				})

				// If the evaluation didn't return the expected value, then the rest of the test is moot
				if !m.In(t).Assert(expectedValue, m.JSONEqual(resp.Value)) {
					require.Fail(t, "evaluation unexpectedly returned wrong value")
				}

				client.FlushEvents(t)

				reason := expectedReason
				if isBadFlag {
					reason = ldreason.NewEvalReasonError(ldreason.EvalErrorMalformedFlag)
				}
				matchFeatureEvent := IsValidFeatureEventWithConditions(
					false, context,
					m.JSONProperty("key").Should(m.Equal(flag.Key)),
					m.JSONProperty("version").Should(m.Equal(flag.Version)),
					m.JSONProperty("value").Should(m.JSONEqual(expectedValue)),
					m.JSONOptProperty("variation").Should(m.JSONEqual(expectedVariation)),
					maybeReason(withReason, reason),
					m.JSONProperty("default").Should(m.JSONEqual(defaultValues(valueType))),
					JSONPropertyNullOrAbsent("prereqOf"),
				)

				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.ItemsInAnyOrder(
					IsIndexEventForContext(context),
					matchFeatureEvent,
					EventHasKind("summary"),
				))
			})
		}
	}

	t.Run("full feature event for tracked flag", func(t *ldtest.T) {
		contextCategories := data.NewContextFactoriesForSingleAndMultiKind("doServerSideFeatureEventTests")
		for _, withReason := range []bool{false, true} {
			t.Run(h.IfElse(withReason, "with reason", "without reason"), func(t *ldtest.T) {
				for _, contextCategory := range contextCategories {
					t.Run(contextCategory.Description(), func(t *ldtest.T) {
						for _, isBadFlag := range []bool{false, true} {
							t.Run(h.IfElse(isBadFlag, "malformed flag", "valid flag"), func(t *ldtest.T) {
								doFeatureEventTest(t, contextCategory, withReason, isBadFlag)
							})
						}
					})
				}
			})
		}
	})

	t.Run("disable full feature event for tracked flag through sampling", func(t *ldtest.T) {
		t.RequireCapability(servicedef.CapabilityEventSampling)

		context := ldcontext.New("example")

		_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
			FlagKey:      zeroSamplingRatioFlag.Key,
			Context:      o.Some(context),
			ValueType:    servicedef.ValueTypeBool,
			DefaultValue: ldvalue.Bool(false),
			Detail:       false,
		})
		client.FlushEvents(t)

		payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
		m.In(t).Assert(payload, m.ItemsInAnyOrder(
			IsIndexEventForContext(context),
			IsSummaryEvent(),
		))
	})

	t.Run("evaluating all flags generates no events", func(t *ldtest.T) {
		_ = client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
			Context: o.Some(contexts.NextUniqueContext()),
		})
		client.FlushEvents(t)
		events.ExpectNoAnalyticsEvents(t, time.Millisecond*200)
	})
}

func doServerSideDebugEventTests(t *ldtest.T) {
	// These tests could misbehave if the system clocks of the host that's running the test harness
	// and the host that's running the test service are out of sync by at least an hour. However,
	// in normal usage those are the same host.

	valueFactories := data.MakeValueFactoriesBySDKValueType(2)
	flagValues, defaultValues := valueFactories[0], valueFactories[1]
	contexts := data.NewContextFactory("doServerSideDebugEventTests")
	expectedReason := ldreason.NewEvalReasonFallthrough()

	doDebugTest := func(
		t *ldtest.T,
		shouldSeeDebugEvent bool,
		flagDebugUntil time.Time,
		lastKnownTimeFromLD time.Time,
	) {
		flags := data.NewFlagFactory(
			"flag",
			flagValues,
			data.FlagShouldProduceThisEvalReason(expectedReason),
			data.FlagShouldHaveDebuggingEnabledUntil(flagDebugUntil),
		)
		dataBuilder := mockld.NewServerSDKDataBuilder()
		for _, valueType := range getValueTypesToTest(t) {
			dataBuilder.Flag(flags.MakeFlagForValueType(valueType))
		}
		dataSource := NewSDKDataSource(t, dataBuilder.Build())

		events := NewSDKEventSink(t)
		if !lastKnownTimeFromLD.IsZero() {
			events.Service().SetHostTimeOverride(lastKnownTimeFromLD)
		}

		client := NewSDKClient(t, dataSource, events)

		if !lastKnownTimeFromLD.IsZero() {
			// In this scenario, we want the SDK to be aware of the LD host's clock because it
			// has seen a Date header in an event post response. Send an unimportant event so
			// the SDK will see a response before we do the rest of the test.
			client.SendIdentifyEvent(t, contexts.NextUniqueContext())
			client.FlushEvents(t)
			_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout)

			// Hacky arbitrary sleep to avoid a race condition where the test code runs fast enough
			// that the SDK has not had a chance to process the HTTP response yet - the fact that
			// we've received the event payload from them doesn't mean the SDK has done that work
			time.Sleep(time.Millisecond * 10)
		}

		for _, withReasons := range []bool{false, true} {
			t.Run(h.IfElse(withReasons, "with reasons", "without reasons"), func(t *ldtest.T) {
				for _, valueType := range getValueTypesToTest(t) {
					t.Run(testDescFromType(valueType), func(t *ldtest.T) {
						context := contexts.NextUniqueContext()
						flag := flags.ReuseFlagForValueType(valueType)
						result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
							FlagKey:      flag.Key,
							Context:      o.Some(context),
							ValueType:    valueType,
							DefaultValue: defaultValues(valueType),
							Detail:       withReasons,
						})
						m.In(t).Assert(result.Value, m.JSONEqual(flagValues(valueType)))

						client.FlushEvents(t)
						payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

						if shouldSeeDebugEvent {
							matchDebugEvent := m.AllOf(
								JSONPropertyKeysCanOnlyBe("kind", "creationDate", "key", "context",
									"version", "value", "variation", "reason", "default"),
								IsDebugEvent(),
								HasAnyCreationDate(),
								m.JSONProperty("key").Should(m.Equal(flag.Key)),
								HasContextObjectWithMatchingKeys(context),
								m.JSONProperty("version").Should(m.Equal(flag.Version)),
								m.JSONProperty("value").Should(m.JSONEqual(result.Value)),
								m.JSONProperty("variation").Should(m.Equal(0)),
								maybeReason(withReasons, expectedReason),
								m.JSONProperty("default").Should(m.JSONEqual(defaultValues(valueType))),
							)
							m.In(t).Assert(payload, m.ItemsInAnyOrder(
								IsIndexEventForContext(context),
								matchDebugEvent,
								EventHasKind("summary"),
							))
						} else {
							m.In(t).Assert(payload, m.ItemsInAnyOrder(
								IsIndexEventForContext(context),
								EventHasKind("summary"),
							))
						}
					})
				}
			})
		}
	}

	doDebugEventTestCases(t, doDebugTest)

	t.Run("index sampling can disable debug event", func(t *ldtest.T) {
		t.RequireCapability(servicedef.CapabilityEventSampling)

		zeroSamplingRatioFlag := ldbuilders.NewFlagBuilder("zero-sampling-ratio").Version(1).
			On(true).Variations(ldvalue.Bool(true), ldvalue.Bool(false)).
			FallthroughVariation(0).
			SamplingRatio(0).Build()

		dataBuilder := mockld.NewServerSDKDataBuilder()
		dataBuilder.Flag(zeroSamplingRatioFlag)
		dataSource := NewSDKDataSource(t, dataBuilder.Build())

		events := NewSDKEventSink(t)
		client := NewSDKClient(t, dataSource, events)

		context := contexts.NextUniqueContext()
		result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
			FlagKey:      zeroSamplingRatioFlag.Key,
			Context:      o.Some(context),
			ValueType:    servicedef.ValueTypeBool,
			DefaultValue: ldvalue.Bool(false),
			Detail:       false,
		})
		m.In(t).Assert(result.Value, m.JSONEqual(ldvalue.Bool(true)))

		client.FlushEvents(t)
		payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		m.In(t).Assert(payload, m.ItemsInAnyOrder(
			IsIndexEventForContext(context),
			EventHasKind("summary"),
		))
	})
}

func doDebugEventTestCases(
	t *ldtest.T,
	doDebugTest func(
		t *ldtest.T,
		shouldSeeDebugEvent bool,
		flagDebugUntil time.Time,
		lastKnownTimeFromLD time.Time,
	),
) {
	shouldSeeDebugEvent := func(t *ldtest.T, debugUntil time.Time, lastKnownTimeFromLD time.Time) {
		doDebugTest(t, true, debugUntil, lastKnownTimeFromLD)
	}
	shouldNotSeeDebugEvent := func(t *ldtest.T, debugUntil time.Time, lastKnownTimeFromLD time.Time) {
		doDebugTest(t, false, debugUntil, lastKnownTimeFromLD)
	}

	t.Run("should see debug event", func(t *ldtest.T) {
		t.Run("debugEventsUntilDate is after SDK time", func(t *ldtest.T) {
			futureDebugUntil := time.Now().Add(time.Hour)
			t.Run("SDK does not know LD time", func(t *ldtest.T) {
				shouldSeeDebugEvent(t, futureDebugUntil, time.Time{})
			})
			t.Run("SDK knows LD time is before debugEventsUntilDate", func(t *ldtest.T) {
				shouldSeeDebugEvent(t, futureDebugUntil, futureDebugUntil.Add(-time.Minute))
			})
		})
	})

	t.Run("should not see debug event", func(t *ldtest.T) {
		t.Run("debugEventsUntilDate is before SDK time", func(t *ldtest.T) {
			pastDebugUntil := time.Now().Add(-time.Hour)
			t.Run("SDK does not know LD time", func(t *ldtest.T) {
				shouldNotSeeDebugEvent(t, pastDebugUntil, time.Time{})
			})
			t.Run("SDK knows LD time is before debugEventsUntilDate", func(t *ldtest.T) {
				shouldNotSeeDebugEvent(t, pastDebugUntil, pastDebugUntil.Add(-time.Minute))
			})
			t.Run("SDK knows LD time is after debugEventsUntilDate", func(t *ldtest.T) {
				shouldNotSeeDebugEvent(t, pastDebugUntil, pastDebugUntil.Add(time.Minute))
			})
		})
		t.Run("debugEventsUntilDate is after SDK time", func(t *ldtest.T) {
			futureDebugUntil := time.Now().Add(time.Hour)
			t.Run("SDK knows LD time is after debugEventsUntilDate", func(t *ldtest.T) {
				shouldNotSeeDebugEvent(t, futureDebugUntil, futureDebugUntil.Add(time.Minute))
			})
		})
	})
}

func doServerSideFeaturePrerequisiteEventTests(t *ldtest.T) {
	// The test logic for this is *almost* exactly the same for PHP as for other server-side SDKs
	// (the only difference is the absence of index and summary events), so we reuse the same
	// function.
	isPHP := t.Capabilities().Has(servicedef.CapabilityPHP)

	context := ldcontext.New("user-key")

	expectedValue1 := ldvalue.String("value1")
	expectedPrereqValue2 := ldvalue.String("ok2")
	expectedPrereqValue3 := ldvalue.String("ok3")
	flag1 := ldbuilders.NewFlagBuilder("flag1").Version(100).
		On(true).OffVariation(0).FallthroughVariation(1).
		AddPrerequisite("flag2", 2).
		Variations(dummyValue0, expectedValue1).
		TrackEvents(true).
		Build()
	flag2 := ldbuilders.NewFlagBuilder("flag2").Version(200).
		On(true).OffVariation(0).FallthroughVariation(0).
		AddPrerequisite("flag3", 3).
		AddTarget(2, "user-key"). // this 2 matches the 2 in flag1's prerequisites
		Variations(dummyValue0, dummyValue1, expectedPrereqValue2).
		TrackEvents(true).
		Build()
	flag3 := ldbuilders.NewFlagBuilder("flag3").Version(300).
		On(true).OffVariation(0).FallthroughVariation(0).
		AddRule(ldbuilders.NewRuleBuilder().ID("rule1").
			Variation(3). // this 3 matches the 3 in flag2's prerequisites
			Clauses(ldbuilders.Clause(ldattr.KeyAttr, ldmodel.OperatorIn, ldvalue.String(context.Key())))).
		Variations(dummyValue0, dummyValue1, dummyValue2, expectedPrereqValue3).
		TrackEvents(true).
		Build()

	for _, withReason := range []bool{false, true} {
		t.Run(h.IfElse(withReason, "with reasons", "without reasons"), func(t *ldtest.T) {
			dataBuilder := mockld.NewServerSDKDataBuilder()
			dataBuilder.Flag(flag1, flag2, flag3)

			dataSource := NewSDKDataSource(t, dataBuilder.Build())
			events := NewSDKEventSink(t)
			client := NewSDKClient(t, dataSource, events)

			result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
				FlagKey:      flag1.Key,
				Context:      o.Some(context),
				ValueType:    servicedef.ValueTypeString,
				DefaultValue: ldvalue.String("default"),
				Detail:       withReason,
			})
			m.In(t).Assert(result.Value, m.JSONEqual(expectedValue1))

			client.FlushEvents(t)
			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

			eventMatchers := []m.Matcher{
				IsValidFeatureEventWithConditions(
					isPHP, context,
					m.JSONProperty("key").Should(m.Equal(flag1.Key)),
					m.JSONProperty("version").Should(m.Equal(flag1.Version)),
					m.JSONProperty("value").Should(m.Equal("value1")),
					m.JSONProperty("variation").Should(m.Equal(1)),
					maybeReason(withReason, ldreason.NewEvalReasonFallthrough()),
					JSONPropertyNullOrAbsent("prereqOf"),
				),
				IsValidFeatureEventWithConditions(
					isPHP, context,
					m.JSONProperty("key").Should(m.Equal(flag2.Key)),
					m.JSONProperty("version").Should(m.Equal(flag2.Version)),
					m.JSONProperty("value").Should(m.Equal("ok2")),
					m.JSONProperty("variation").Should(m.Equal(2)),
					maybeReason(withReason, ldreason.NewEvalReasonTargetMatch()),
					JSONPropertyNullOrAbsent("default"),
					m.JSONOptProperty("prereqOf").Should(m.Equal("flag1")),
				),
				IsValidFeatureEventWithConditions(
					isPHP, context,
					m.JSONProperty("key").Should(m.Equal(flag3.Key)),
					m.JSONProperty("version").Should(m.Equal(flag3.Version)),
					m.JSONProperty("value").Should(m.Equal("ok3")),
					m.JSONProperty("variation").Should(m.Equal(3)),
					maybeReason(withReason, ldreason.NewEvalReasonRuleMatch(0, "rule1")),
					JSONPropertyNullOrAbsent("default"),
					m.JSONOptProperty("prereqOf").Should(m.Equal("flag2")),
				),
			}
			if !isPHP {
				eventMatchers = append(eventMatchers, IsIndexEventForContext(context), IsSummaryEvent())
			}
			m.In(t).Assert(payload, m.ItemsInAnyOrder(eventMatchers...))
		})
	}

	t.Run("evaluating all flags generates no events", func(t *ldtest.T) {
		dataBuilder := mockld.NewServerSDKDataBuilder()
		dataBuilder.Flag(flag1, flag2, flag3)

		dataSource := NewSDKDataSource(t, dataBuilder.Build())
		events := NewSDKEventSink(t)
		client := NewSDKClient(t, dataSource, events)

		_ = client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
			Context: o.Some(context),
		})
		client.FlushEvents(t)
		events.ExpectNoAnalyticsEvents(t, time.Millisecond*200)
	})
}

func maybeReason(withReason bool, reason ldreason.EvaluationReason) m.Matcher {
	return h.IfElse(withReason,
		m.JSONProperty("reason").Should(m.JSONEqual(reason)),
		JSONPropertyNullOrAbsent("reason"))
}
