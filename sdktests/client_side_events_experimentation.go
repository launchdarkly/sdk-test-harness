package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

func doClientSideExperimentationEventTests(t *ldtest.T) {
	// An evaluation that involves an experiment (via either a rule match or a fallthrough) should always
	// generate a full feature event even if event tracking is not enabled for the flag. Also, the event
	// will contain an evaluation reason in this case regardless of whether the application requested one.

	// The client-side version of this test is much simpler than the server-side one, because the results
	// of the evaluation have already been provided by LaunchDarkly; if an experiment was involved, then
	// "trackEvents" and "trackReason" will both be true and "reason" will always be set. So we are just
	// verifying that the SDK correctly copies the reason into the event in this case.

	expectedValue := ldvalue.String("good")
	expectedVariation := 1
	flagVersion := 100
	defaultValue := ldvalue.String("default")
	user := lduser.NewUser("user-key")

	for _, expectedReason := range []ldreason.EvaluationReason{
		ldreason.NewEvalReasonFallthroughExperiment(true),
		ldreason.NewEvalReasonRuleMatchExperiment(1, "ruleid", true),
	} {
		t.Run(string(expectedReason.GetKind()), func(t *ldtest.T) {
			flagKey := "flag-key"
			data := mockld.NewClientSDKDataBuilder().
				Flag(flagKey, mockld.ClientSDKFlag{
					Version:     flagVersion,
					Value:       expectedValue,
					Variation:   o.Some(expectedVariation),
					Reason:      o.Some(expectedReason),
					TrackEvents: true,
					TrackReason: true,
				}).
				Build()

			dataSource := NewSDKDataSource(t, data)
			eventSink := NewSDKEventSink(t)
			client := NewSDKClient(t,
				WithClientSideConfig(servicedef.SDKConfigClientSideParams{
					InitialUser: user,
				}),
				dataSource,
				eventSink,
			)

			result := basicEvaluateFlag(t, client, flagKey, user, defaultValue)
			m.In(t).Assert(result, m.JSONEqual(expectedValue))

			client.FlushEvents(t)
			payload := eventSink.ExpectAnalyticsEvents(t, time.Second)

			matchFeatureEvent := IsValidFeatureEventWithConditions(
				m.JSONProperty("key").Should(m.Equal(flagKey)),
				HasUserKeyProperty(user.GetKey()),
				HasNoUserObject(),
				m.JSONProperty("version").Should(m.Equal(flagVersion)),
				m.JSONProperty("value").Should(m.JSONEqual(expectedValue)),
				m.JSONProperty("variation").Should(m.Equal(expectedVariation)),
				m.JSONProperty("reason").Should(m.JSONEqual(expectedReason)),
				m.JSONProperty("default").Should(m.JSONEqual(defaultValue)),
				JSONPropertyNullOrAbsent("prereqOf"),
			)

			m.In(t).Assert(payload, m.ItemsInAnyOrder(
				IsIdentifyEvent(),
				matchFeatureEvent,
				IsSummaryEvent(),
			))
		})
	}
}
