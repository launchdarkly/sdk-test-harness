package sdktests

import (
	"time"

	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/sdk-test-harness/v2/data"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

// This file is very similar to server_side_events_eval.go, except:
//
// - The test data generation works differently because of the different flag model.
// - We're not using a unique user per evaluation.
// - There are no prerequisite events.

func doClientSideFeatureEventTests(t *ldtest.T) {
	valueFactories := data.MakeValueFactoriesBySDKValueType(2)
	flagValues, defaultValues := valueFactories[0], valueFactories[1]
	context := data.NewContextFactory("doClientSideFeatureEventTests").NextUniqueContext()
	expectedReason := ldreason.NewEvalReasonFallthrough()
	untrackedFlags := data.NewClientSideFlagFactory(
		"untracked-flag",
		flagValues,
		data.ClientSideFlagShouldHaveEvalReason(expectedReason),
	)
	trackedFlags := data.NewClientSideFlagFactory(
		"tracked-flag",
		flagValues,
		data.ClientSideFlagShouldHaveEvalReason(expectedReason),
		data.ClientSideFlagShouldHaveFullEventTracking,
	)

	dataBuilder := mockld.NewClientSDKDataBuilder()
	for _, valueType := range getValueTypesToTest(t) {
		dataBuilder.FullFlag(untrackedFlags.MakeFlagForValueType(valueType))
		dataBuilder.FullFlag(trackedFlags.MakeFlagForValueType(valueType))
	}

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSinkWithGzip(t, t.Capabilities().Has(servicedef.CapabilityEventGzip))

	client := NewSDKClient(t,
		WithClientSideInitialContext(context),
		dataSource, events)

	client.FlushEvents(t)
	_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout) // discard initial identify event

	t.Run("only summary event for untracked flag", func(t *ldtest.T) {
		for _, valueType := range getValueTypesToTest(t) {
			t.Run(testDescFromType(valueType), func(t *ldtest.T) {
				flag := untrackedFlags.ReuseFlagForValueType(valueType)

				resp := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
					FlagKey:      flag.Key,
					ValueType:    valueType,
					DefaultValue: defaultValues(valueType),
				})

				// If the evaluation didn't return the expected value, then the rest of the test is moot
				if !m.In(t).Assert(flag.Value, m.JSONEqual(resp.Value)) {
					require.Fail(t, "evaluation unexpectedly returned wrong value")
				}

				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.ItemsInAnyOrder(
					IsSummaryEvent(),
				))
			})
		}
	})

	doFeatureEventTest := func(t *ldtest.T, withReason bool) {
		for _, valueType := range getValueTypesToTest(t) {
			t.Run(testDescFromType(valueType), func(t *ldtest.T) {
				flag := trackedFlags.ReuseFlagForValueType(valueType)
				expectedValue := flag.Value
				expectedVariation := flag.Variation
				resp := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
					FlagKey:      flag.Key,
					ValueType:    valueType,
					DefaultValue: defaultValues(valueType),
					Detail:       withReason,
				})

				// If the evaluation didn't return the expected value, then the rest of the test is moot
				if !m.In(t).Assert(expectedValue, m.JSONEqual(resp.Value)) {
					require.Fail(t, "evaluation unexpectedly returned wrong value")
				}

				client.FlushEvents(t)

				matchFeatureEvent := IsValidFeatureEventWithConditions(
					t, false, context,
					m.JSONProperty("key").Should(m.Equal(flag.Key)),
					m.JSONProperty("version").Should(m.Equal(flag.Version)),
					m.JSONProperty("value").Should(m.JSONEqual(expectedValue)),
					m.JSONOptProperty("variation").Should(m.JSONEqual(expectedVariation)),
					maybeReason(withReason, expectedReason),
					m.JSONProperty("default").Should(m.JSONEqual(defaultValues(valueType))),
					JSONPropertyNullOrAbsent("prereqOf"),
				)

				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.ItemsInAnyOrder(
					matchFeatureEvent,
					IsSummaryEvent(),
				))
			})
		}
	}

	t.Run("full feature event for tracked flag", func(t *ldtest.T) {
		for _, withReason := range []bool{false, true} {
			t.Run(h.IfElse(withReason, "with reason", "without reason"), func(t *ldtest.T) {
				doFeatureEventTest(t, withReason)
			})
		}
	})

	if t.Capabilities().Has(servicedef.CapabilityAnonymousRedaction) {
		t.Run("single-kind anonymous context redacts all attributes", func(t *ldtest.T) {
			anonymousFactory := data.NewContextFactory("anonymous", func(b *ldcontext.Builder) {
				b.Anonymous(true)
				b.Name("Example name")
				b.SetString("setup", "Why do programmers always confused Halloween and Christmas?")
				b.SetString("punchline", "Because OCT 31 = DEC 25")
			})

			for _, valueType := range getValueTypesToTest(t) {
				t.Run(testDescFromType(valueType), func(t *ldtest.T) {
					flag := trackedFlags.ReuseFlagForValueType(valueType)
					expectedValue := flag.Value
					anonymousContext := anonymousFactory.NextUniqueContext()

					client.SendIdentifyEvent(t, anonymousContext)
					client.FlushEvents(t)
					_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout) // discard initial identify event

					resp := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
						FlagKey:      flag.Key,
						ValueType:    valueType,
						DefaultValue: defaultValues(valueType),
						Detail:       true,
					})

					// If the evaluation didn't return the expected value, then the rest of the test is moot
					if !m.In(t).Assert(expectedValue, m.JSONEqual(resp.Value)) {
						require.Fail(t, "evaluation unexpectedly returned wrong value")
					}

					client.FlushEvents(t)

					expectedContext := ldcontext.NewBuilderFromContext(anonymousContext).
						SetValue("name", ldvalue.Null()).
						SetValue("setup", ldvalue.Null()).
						SetValue("punchline", ldvalue.Null()).
						Build()

					matcher := JSONMatchesEventContext(expectedContext, map[string][]string{"user": {"name", "setup", "punchline"}})

					payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
					m.In(t).Assert(payload, m.ItemsInAnyOrder(
						IsValidFeatureEventWithConditions(t, false, anonymousContext, m.JSONProperty("context").Should(matcher)),
						EventHasKind("summary"),
					))
				})
			}
		})

		t.Run("multi-kind with anonymous context redacts attributes appropriately", func(t *ldtest.T) {
			userContextFactory := data.NewContextFactory("user", func(b *ldcontext.Builder) {
				b.Anonymous(true)
				b.Kind("user")
				b.Name("User name")
				b.SetString("setup", "Why do programmers always confused Halloween and Christmas?")
				b.SetString("punchline", "Because OCT 31 = DEC 25")
			})
			orgContextFactory := data.NewContextFactory("org", func(b *ldcontext.Builder) {
				b.Name("Org name")
				b.Kind("org")
				b.SetString("setup", "Why did the edge server go bankrupt?")
				b.SetString("punchline", "Because it ran out of cache")
			})

			for _, valueType := range getValueTypesToTest(t) {
				t.Run(testDescFromType(valueType), func(t *ldtest.T) {
					userContext := userContextFactory.NextUniqueContext()
					orgContext := orgContextFactory.NextUniqueContext()

					multiContext := ldcontext.NewMultiBuilder().Add(userContext).Add(orgContext).Build()

					client.SendIdentifyEvent(t, multiContext)
					client.FlushEvents(t)
					_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout) // discard initial identify event

					flag := trackedFlags.ReuseFlagForValueType(valueType)
					expectedValue := flagValues(valueType)
					resp := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
						FlagKey:      flag.Key,
						ValueType:    valueType,
						DefaultValue: defaultValues(valueType),
						Detail:       false,
					})

					// If the evaluation didn't return the expected value, then the rest of the test is moot
					if !m.In(t).Assert(expectedValue, m.JSONEqual(resp.Value)) {
						require.Fail(t, "evaluation unexpectedly returned wrong value")
					}

					client.FlushEvents(t)

					expectedUser := ldcontext.NewBuilderFromContext(userContext).
						SetValue("name", ldvalue.Null()).
						SetValue("setup", ldvalue.Null()).
						SetValue("punchline", ldvalue.Null()).
						Build()

					expectedMultiKind := ldcontext.NewMultiBuilder().Add(expectedUser).Add(orgContext).Build()

					matcher := JSONMatchesEventContext(expectedMultiKind, map[string][]string{"user": {"name", "setup", "punchline"}})

					payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
					m.In(t).Assert(payload, m.ItemsInAnyOrder(
						IsValidFeatureEventWithConditions(t, false, multiContext, m.JSONProperty("context").Should(matcher)),
						EventHasKind("summary"),
					))
				})
			}
		})

		// Restore the client to the effective context object prior to this block.
		client.SendIdentifyEvent(t, context)
		client.FlushEvents(t)
		_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout)
	}

	t.Run("evaluating all flags generates no events", func(t *ldtest.T) {
		_ = client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{})
		client.FlushEvents(t)
		events.ExpectNoAnalyticsEvents(t, time.Millisecond*200)
	})
}

func doClientSideDebugEventTests(t *ldtest.T) {
	// These tests could misbehave if the system clocks of the host that's running the test harness
	// and the host that's running the test service are out of sync by at least an hour. However,
	// in normal usage those are the same host.

	valueFactories := data.MakeValueFactoriesBySDKValueType(2)
	flagValues, defaultValues := valueFactories[0], valueFactories[1]
	contexts := data.NewContextFactory("doClientSideDebugEventTests")
	expectedReason := ldreason.NewEvalReasonFallthrough()

	doDebugTest := func(
		t *ldtest.T,
		shouldSeeDebugEvent bool,
		flagDebugUntil time.Time,
		lastKnownTimeFromLD time.Time,
	) {
		context := contexts.NextUniqueContext()
		flags := data.NewClientSideFlagFactory(
			"flag",
			flagValues,
			data.ClientSideFlagShouldHaveEvalReason(expectedReason),
			data.ClientSideFlagShouldHaveDebuggingEnabledUntil(flagDebugUntil),
		)
		dataBuilder := mockld.NewClientSDKDataBuilder()
		for _, valueType := range getValueTypesToTest(t) {
			dataBuilder.FullFlag(flags.MakeFlagForValueType(valueType))
		}
		dataSource := NewSDKDataSource(t, dataBuilder.Build())

		events := NewSDKEventSinkWithGzip(t, t.Capabilities().Has(servicedef.CapabilityEventGzip))
		if !lastKnownTimeFromLD.IsZero() {
			events.Service().SetHostTimeOverride(lastKnownTimeFromLD)
		}

		client := NewSDKClient(t,
			WithClientSideInitialContext(context),
			dataSource, events)

		client.FlushEvents(t)
		_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout) // discard initial identify event
		// note, this initial flush also causes the SDK to see the Date header in the mock event service's response

		if !lastKnownTimeFromLD.IsZero() {
			// Hacky arbitrary sleep to avoid a race condition where the test code runs fast enough
			// that the SDK has not had a chance to process the HTTP response yet - the fact that
			// we've received the event payload from them doesn't mean the SDK has done that work
			time.Sleep(time.Millisecond * 10)
		}

		for _, withReasons := range []bool{false, true} {
			t.Run(h.IfElse(withReasons, "with reasons", "without reasons"), func(t *ldtest.T) {
				for _, valueType := range getValueTypesToTest(t) {
					t.Run(testDescFromType(valueType), func(t *ldtest.T) {
						flag := flags.ReuseFlagForValueType(valueType)
						result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
							FlagKey:      flag.Key,
							ValueType:    valueType,
							DefaultValue: defaultValues(valueType),
							Detail:       withReasons,
						})
						m.In(t).Assert(result.Value, m.JSONEqual(flag.Value))

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
								m.JSONProperty("variation").Should(m.JSONEqual(flag.Variation)),
								maybeReason(withReasons, expectedReason),
								m.JSONProperty("default").Should(m.JSONEqual(defaultValues(valueType))),
							)
							m.In(t).Assert(payload, m.ItemsInAnyOrder(
								matchDebugEvent,
								EventHasKind("summary"),
							))
						} else {
							m.In(t).Assert(payload, m.ItemsInAnyOrder(
								EventHasKind("summary"),
							))
						}
					})
				}
			})
		}
	}

	doDebugEventTestCases(t, doDebugTest)
}
