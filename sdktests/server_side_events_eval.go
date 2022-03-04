package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"

	"github.com/stretchr/testify/require"
)

func doServerSideFeatureEventTests(t *ldtest.T) {
	flagValues := FlagValueByTypeFactory()
	defaultValues := DefaultValueByTypeFactory()
	users := NewUserFactory("doServerSideFeatureEventTests")
	expectedReason := ldreason.NewEvalReasonFallthrough()
	untrackedFlags := FlagFactoryForValueTypes{
		KeyPrefix:    "untracked-flag-",
		ValueFactory: flagValues,
		Reason:       expectedReason,
	}
	trackedFlags := FlagFactoryForValueTypes{
		KeyPrefix:      "tracked-flag-",
		ValueFactory:   flagValues,
		BuilderActions: func(b *ldbuilders.FlagBuilder) { b.TrackEvents(true) },
		Reason:         expectedReason,
	}
	malformedFlag := ldbuilders.NewFlagBuilder("bad-flag").Version(1).
		On(false).OffVariation(-1).TrackEvents(true).Build()

	dataBuilder := mockld.NewServerSDKDataBuilder()
	for _, valueType := range getValueTypesToTest(t) {
		dataBuilder.Flag(untrackedFlags.ForType(valueType))
		dataBuilder.Flag(trackedFlags.ForType(valueType))
	}
	dataBuilder.Flag(malformedFlag)

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSink(t)

	client := NewSDKClient(t, dataSource, events)

	t.Run("only index + summary event for untracked flag", func(t *ldtest.T) {
		for _, withReason := range []bool{false, true} {
			t.Run(selectString(withReason, "with reasons", "without reasons"), func(t *ldtest.T) {
				for _, valueType := range getValueTypesToTest(t) {
					t.Run(testDescFromType(valueType), func(t *ldtest.T) {
						flag := untrackedFlags.ForType(valueType)
						user := users.NextUniqueUser()

						resp := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
							FlagKey:      flag.Key,
							User:         user,
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
							IsIndexEventForUserKey(user.GetKey()),
							IsSummaryEvent(),
						))
					})
				}
			})
		}
	})

	doFeatureEventTest := func(t *ldtest.T, withReason, isAnonymousUser, isBadFlag bool) {
		for _, valueType := range getValueTypesToTest(t) {
			t.Run(testDescFromType(valueType), func(t *ldtest.T) {
				flag := trackedFlags.ForType(valueType)
				expectedValue := flagValues(valueType)
				expectedVariation := ldvalue.NewOptionalInt(0)
				if isBadFlag {
					flag = malformedFlag
					expectedValue = defaultValues(valueType)
					expectedVariation = ldvalue.OptionalInt{}
				}
				user := users.NextUniqueUserMaybeAnonymous(isAnonymousUser)
				resp := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
					FlagKey:      flag.Key,
					User:         user,
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
					m.JSONProperty("key").Should(m.Equal(flag.Key)),
					HasUserKeyProperty(user.GetKey()),
					HasNoUserObject(),
					m.JSONProperty("version").Should(m.Equal(flag.Version)),
					m.JSONProperty("value").Should(m.JSONEqual(expectedValue)),
					m.JSONOptProperty("variation").Should(m.JSONEqual(expectedVariation)),
					maybeReason(withReason, reason),
					m.JSONProperty("default").Should(m.JSONEqual(defaultValues(valueType))),
					JSONPropertyNullOrAbsent("prereqOf"),
				)

				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.ItemsInAnyOrder(
					IsIndexEventForUserKey(user.GetKey()),
					matchFeatureEvent,
					EventHasKind("summary"),
				))
			})
		}
	}

	t.Run("full feature event for tracked flag", func(t *ldtest.T) {
		for _, withReason := range []bool{false, true} {
			t.Run(selectString(withReason, "with reason", "without reason"), func(t *ldtest.T) {
				for _, isAnonymousUser := range []bool{false, true} {
					t.Run(selectString(isAnonymousUser, "anonymous user", "non-anonymous user"), func(t *ldtest.T) {
						for _, isBadFlag := range []bool{false, true} {
							t.Run(selectString(isBadFlag, "malformed flag", "valid flag"), func(t *ldtest.T) {
								doFeatureEventTest(t, withReason, isAnonymousUser, isBadFlag)
							})
						}
					})
				}
			})
		}
	})
}

func doServerSideDebugEventTests(t *ldtest.T) {
	// These tests could misbehave if the system clocks of the host that's running the test harness
	// and the host that's running the test service are out of sync by at least an hour. However,
	// in normal usage those are the same host.

	flagValues := FlagValueByTypeFactory()
	defaultValues := DefaultValueByTypeFactory()
	users := NewUserFactory("doServerSideDebugEventTests")
	expectedReason := ldreason.NewEvalReasonFallthrough()

	doDebugTest := func(
		t *ldtest.T,
		shouldSeeDebugEvent bool,
		flagDebugUntil time.Time,
		lastKnownTimeFromLD time.Time,
	) {
		flags := FlagFactoryForValueTypes{
			KeyPrefix:    "flag",
			ValueFactory: flagValues,
			Reason:       expectedReason,
			BuilderActions: func(b *ldbuilders.FlagBuilder) {
				b.DebugEventsUntilDate(ldtime.UnixMillisFromTime(flagDebugUntil))
			},
		}
		dataBuilder := mockld.NewServerSDKDataBuilder()
		for _, valueType := range getValueTypesToTest(t) {
			dataBuilder.Flag(flags.ForType(valueType))
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
			client.SendIdentifyEvent(t, users.NextUniqueUser())
			client.FlushEvents(t)
			_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout)

			// Hacky arbitrary sleep to avoid a race condition where the test code runs fast enough
			// that the SDK has not had a chance to process the HTTP response yet - the fact that
			// we've received the event payload from them doesn't mean the SDK has done that work
			time.Sleep(time.Millisecond * 10)
		}

		for _, withReasons := range []bool{false, true} {
			t.Run(selectString(withReasons, "with reasons", "without reasons"), func(t *ldtest.T) {
				for _, valueType := range getValueTypesToTest(t) {
					t.Run(testDescFromType(valueType), func(t *ldtest.T) {
						user := users.NextUniqueUser()
						flag := flags.ForType(valueType)
						result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
							FlagKey:      flag.Key,
							User:         user,
							ValueType:    valueType,
							DefaultValue: defaultValues(valueType),
							Detail:       withReasons,
						})
						m.In(t).Assert(result.Value, m.JSONEqual(flagValues(valueType)))

						client.FlushEvents(t)
						payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

						if shouldSeeDebugEvent {
							matchDebugEvent := m.AllOf(
								JSONPropertyKeysCanOnlyBe("kind", "creationDate", "key", "user",
									"version", "value", "variation", "reason", "default"),
								IsDebugEvent(),
								HasAnyCreationDate(),
								m.JSONProperty("key").Should(m.Equal(flag.Key)),
								HasUserObjectWithKey(user.GetKey()),
								m.JSONProperty("version").Should(m.Equal(flag.Version)),
								m.JSONProperty("value").Should(m.JSONEqual(result.Value)),
								m.JSONProperty("variation").Should(m.Equal(0)),
								maybeReason(withReasons, expectedReason),
								m.JSONProperty("default").Should(m.JSONEqual(defaultValues(valueType))),
							)
							m.In(t).Assert(payload, m.ItemsInAnyOrder(
								IsIndexEventForUserKey(user.GetKey()),
								matchDebugEvent,
								EventHasKind("summary"),
							))
						} else {
							m.In(t).Assert(payload, m.ItemsInAnyOrder(
								IsIndexEventForUserKey(user.GetKey()),
								EventHasKind("summary"),
							))
						}
					})
				}
			})
		}
	}
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
	user := lduser.NewUser("user-key")

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
			Clauses(ldbuilders.Clause(lduser.KeyAttribute, ldmodel.OperatorIn, ldvalue.String(user.GetKey())))).
		Variations(dummyValue0, dummyValue1, dummyValue2, expectedPrereqValue3).
		TrackEvents(true).
		Build()

	for _, withReason := range []bool{false, true} {
		t.Run(selectString(withReason, "with reasons", "without reasons"), func(t *ldtest.T) {
			dataBuilder := mockld.NewServerSDKDataBuilder()
			dataBuilder.Flag(flag1, flag2, flag3)

			dataSource := NewSDKDataSource(t, dataBuilder.Build())
			events := NewSDKEventSink(t)
			client := NewSDKClient(t, dataSource, events)

			result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
				FlagKey:      flag1.Key,
				User:         user,
				ValueType:    servicedef.ValueTypeString,
				DefaultValue: ldvalue.String("default"),
				Detail:       withReason,
			})
			m.In(t).Assert(result.Value, m.JSONEqual(expectedValue1))

			client.FlushEvents(t)
			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

			m.In(t).Assert(payload, m.ItemsInAnyOrder(
				IsIndexEventForUserKey(user.GetKey()),
				IsValidFeatureEventWithConditions(
					m.JSONProperty("key").Should(m.Equal(flag1.Key)),
					HasUserKeyProperty(user.GetKey()),
					HasNoUserObject(),
					m.JSONProperty("version").Should(m.Equal(flag1.Version)),
					m.JSONProperty("value").Should(m.Equal("value1")),
					m.JSONProperty("variation").Should(m.Equal(1)),
					maybeReason(withReason, ldreason.NewEvalReasonFallthrough()),
					JSONPropertyNullOrAbsent("prereqOf"),
				),
				IsValidFeatureEventWithConditions(
					m.JSONProperty("key").Should(m.Equal(flag2.Key)),
					HasUserKeyProperty(user.GetKey()),
					HasNoUserObject(),
					m.JSONProperty("version").Should(m.Equal(flag2.Version)),
					m.JSONProperty("value").Should(m.Equal("ok2")),
					m.JSONProperty("variation").Should(m.Equal(2)),
					maybeReason(withReason, ldreason.NewEvalReasonTargetMatch()),
					JSONPropertyNullOrAbsent("default"),
					m.JSONOptProperty("prereqOf").Should(m.Equal("flag1")),
				),
				IsValidFeatureEventWithConditions(
					m.JSONProperty("key").Should(m.Equal(flag3.Key)),
					HasUserKeyProperty(user.GetKey()),
					HasNoUserObject(),
					m.JSONProperty("version").Should(m.Equal(flag3.Version)),
					m.JSONProperty("value").Should(m.Equal("ok3")),
					m.JSONProperty("variation").Should(m.Equal(3)),
					maybeReason(withReason, ldreason.NewEvalReasonRuleMatch(0, "rule1")),
					JSONPropertyNullOrAbsent("default"),
					m.JSONOptProperty("prereqOf").Should(m.Equal("flag2")),
				),
				IsSummaryEvent(),
			))
		})
	}
}

func maybeReason(withReason bool, reason ldreason.EvaluationReason) m.Matcher {
	return conditionalMatcher(withReason,
		m.JSONProperty("reason").Should(m.JSONEqual(reason)),
		JSONPropertyNullOrAbsent("reason"))
}
