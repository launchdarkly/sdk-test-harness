package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
)

func doServerSideExperimentationEventTests(t *ldtest.T) {
	// An evaluation that involves an experiment (via either a rule match or a fallthrough) should always
	// generate a full feature event even if event tracking is not enabled for the flag. Also, the event
	// will contain an evaluation reason in this case regardless of whether the application requested one.

	expectedValue := ldvalue.String("good")
	expectedVariation := 1
	wrongValue := ldvalue.String("bad")
	defaultValue := ldvalue.String("default")
	rollout := ldmodel.VariationOrRollout{
		Rollout: ldmodel.Rollout{
			Kind: ldmodel.RolloutKindExperiment,
			Variations: []ldmodel.WeightedVariation{
				{
					Variation: expectedVariation,
					Weight:    100000,
				},
			},
		},
	}
	user := lduser.NewUser("user-key")

	scenarios := []struct {
		name           string
		flagConfig     func(*ldbuilders.FlagBuilder)
		expectedReason ldreason.EvaluationReason
	}{
		{
			name: "experiment in rule",
			flagConfig: func(f *ldbuilders.FlagBuilder) {
				f.AddRule(ldbuilders.NewRuleBuilder().ID("rule1").VariationOrRollout(rollout))
			},
			expectedReason: ldreason.NewEvalReasonRuleMatchExperiment(0, "rule1", true),
		},
		{
			name: "experiment in fallthrough",
			flagConfig: func(f *ldbuilders.FlagBuilder) {
				f.Fallthrough(rollout)
			},
			expectedReason: ldreason.NewEvalReasonFallthroughExperiment(true),
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *ldtest.T) {
			builder := ldbuilders.NewFlagBuilder("flag-key").Version(1).On(true).
				Variations(wrongValue, expectedValue)
			scenario.flagConfig(builder)
			flag := builder.Build()
			data := mockld.NewServerSDKDataBuilder().Flag(flag).Build()

			dataSource := NewSDKDataSource(t, data)
			eventSink := NewSDKEventSink(t)
			client := NewSDKClient(t, dataSource, eventSink)

			result := basicEvaluateFlag(t, client, flag.Key, user, defaultValue)
			m.In(t).Assert(result, m.JSONEqual(expectedValue))

			client.FlushEvents(t)
			payload := eventSink.ExpectAnalyticsEvents(t, time.Second)

			matchFeatureEvent := IsValidFeatureEventWithConditions(
				m.JSONProperty("key").Should(m.Equal(flag.Key)),
				HasUserKeyProperty(user.GetKey()),
				HasNoUserObject(),
				m.JSONProperty("version").Should(m.Equal(flag.Version)),
				m.JSONProperty("value").Should(m.JSONEqual(expectedValue)),
				m.JSONProperty("variation").Should(m.Equal(expectedVariation)),
				m.JSONProperty("reason").Should(m.JSONEqual(scenario.expectedReason)),
				m.JSONProperty("default").Should(m.JSONEqual(defaultValue)),
				JSONPropertyNullOrAbsent("prereqOf"),
			)

			m.In(t).Assert(payload, m.ItemsInAnyOrder(
				IsIndexEventForUserKey(user.GetKey()),
				matchFeatureEvent,
				IsSummaryEvent(),
			))
		})
	}
}
