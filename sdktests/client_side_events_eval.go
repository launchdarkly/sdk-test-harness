package sdktests

import (
	"time"

	h "github.com/launchdarkly/sdk-test-harness/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"

	"github.com/stretchr/testify/require"
)

func doClientSideFeatureEventTests(t *ldtest.T) {
	flagValues := FlagValueByTypeFactory()
	defaultValues := DefaultValueByTypeFactory()
	user := NewUserFactory("doClientSideFeatureEventTests").NextUniqueUser()
	expectedReason := ldreason.NewEvalReasonFallthrough()
	untrackedFlags := ClientSideFlagFactoryForValueTypes{
		KeyPrefix:    "untracked-flag-",
		ValueFactory: flagValues,
		Reason:       expectedReason,
	}
	trackedFlags := ClientSideFlagFactoryForValueTypes{
		KeyPrefix:      "tracked-flag-",
		ValueFactory:   flagValues,
		BuilderActions: func(f *mockld.ClientSDKFlagWithKey) { f.TrackEvents = true },
		Reason:         expectedReason,
	}

	dataBuilder := mockld.NewClientSDKDataBuilder()
	for _, valueType := range getValueTypesToTest(t) {
		dataBuilder.FullFlag(untrackedFlags.ForType(valueType))
		dataBuilder.FullFlag(trackedFlags.ForType(valueType))
	}

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSink(t)

	client := NewSDKClient(t,
		WithClientSideConfig(servicedef.SDKConfigClientSideParams{
			InitialUser: user,
		}),
		dataSource, events)

	client.FlushEvents(t)
	_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout) // discard initial identify event

	t.Run("only summary event for untracked flag", func(t *ldtest.T) {
		for _, valueType := range getValueTypesToTest(t) {
			t.Run(testDescFromType(valueType), func(t *ldtest.T) {
				flag := untrackedFlags.ForType(valueType)

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
				flag := trackedFlags.ForType(valueType)
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
					false, false, user,
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

	flagValues := FlagValueByTypeFactory()
	defaultValues := DefaultValueByTypeFactory()
	users := NewUserFactory("doClientSideDebugEventTests")
	expectedReason := ldreason.NewEvalReasonFallthrough()

	doDebugTest := func(
		t *ldtest.T,
		shouldSeeDebugEvent bool,
		flagDebugUntil time.Time,
		lastKnownTimeFromLD time.Time,
	) {
		user := users.NextUniqueUser()
		flags := ClientSideFlagFactoryForValueTypes{
			KeyPrefix:    "flag",
			ValueFactory: flagValues,
			Reason:       expectedReason,
			BuilderActions: func(f *mockld.ClientSDKFlagWithKey) {
				f.DebugEventsUntilDate = o.Some(ldtime.UnixMillisFromTime(flagDebugUntil))
			},
		}
		dataBuilder := mockld.NewClientSDKDataBuilder()
		for _, valueType := range getValueTypesToTest(t) {
			dataBuilder.FullFlag(flags.ForType(valueType))
		}
		dataSource := NewSDKDataSource(t, dataBuilder.Build())

		events := NewSDKEventSink(t)
		if !lastKnownTimeFromLD.IsZero() {
			events.Service().SetHostTimeOverride(lastKnownTimeFromLD)
		}

		client := NewSDKClient(t,
			WithClientSideConfig(servicedef.SDKConfigClientSideParams{
				InitialUser: user,
			}),
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
						flag := flags.ForType(valueType)
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
								JSONPropertyKeysCanOnlyBe("kind", "creationDate", "key", "user",
									"version", "value", "variation", "reason", "default"),
								IsDebugEvent(),
								HasAnyCreationDate(),
								m.JSONProperty("key").Should(m.Equal(flag.Key)),
								HasUserObjectWithKey(user.GetKey()),
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
