package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

func doServerSideExperimentationEventTests(t *ldtest.T) {
	// An evaluation that involves an experiment (via either a rule match or a fallthrough) should always
	// generate a full feature event even if event tracking is not enabled for the flag. Also, the event
	// will contain an evaluation reason in this case regardless of whether the application requested one.

	// The test logic for this is *almost* exactly the same for PHP as for other server-side SDKs
	// (the only difference is the absence of index and summary events), so we reuse the same
	// function.
	isPHP := t.Capabilities().Has(servicedef.CapabilityPHP)

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
	context := ldcontext.New("user-key")

	scenarios := []struct {
		name           string
		flagConfig     func(*ldbuilders.FlagBuilder)
		expectedReason string
	}{
		{
			name: "experiment in rule",
			flagConfig: func(f *ldbuilders.FlagBuilder) {
				f.AddRule(ldbuilders.NewRuleBuilder().ID("rule1").VariationOrRollout(rollout))
			},
			expectedReason: `{"kind": "RULE_MATCH", "ruleIndex": 0, "ruleId": "rule1", "inExperiment": true}`,
		},
		{
			name: "experiment in fallthrough",
			flagConfig: func(f *ldbuilders.FlagBuilder) {
				f.Fallthrough(rollout)
			},
			expectedReason: `{"kind": "FALLTHROUGH", "inExperiment": true}`,
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

			result := basicEvaluateFlag(t, client, flag.Key, context, defaultValue)
			m.In(t).Assert(result, m.JSONEqual(expectedValue))

			client.FlushEvents(t)
			payload := eventSink.ExpectAnalyticsEvents(t, time.Second)

			eventMatchers := []m.Matcher{
				IsValidFeatureEventWithConditions(
					t, isPHP, context,
					m.JSONProperty("key").Should(m.Equal(flag.Key)),
					m.JSONProperty("version").Should(m.Equal(flag.Version)),
					m.JSONProperty("value").Should(m.JSONEqual(expectedValue)),
					m.JSONProperty("variation").Should(m.Equal(expectedVariation)),
					m.JSONProperty("reason").Should(m.JSONStrEqual(scenario.expectedReason)),
					m.JSONProperty("default").Should(m.JSONEqual(defaultValue)),
					JSONPropertyNullOrAbsent("prereqOf"),
				),
			}
			if !isPHP {
				eventMatchers = append(eventMatchers, IsIndexEventForContext(context), IsSummaryEvent())
			}
			m.In(t).Assert(payload, m.ItemsInAnyOrder(eventMatchers...))
		})
	}

	// A rule with trackEvents, or a fallthrough with trackEventsFallthrough, has the same effect as
	// an experiment: a full feature event with a reason, even if the flag is not tracked and the
	// application did not ask for a reason. It must not affect the other path through the flag.
	//
	// An experiment rollout only counts as an experiment if the context is actually in the
	// experiment: not if it lands in an untracked bucket, and not if it lacks the context kind
	// that the rollout is for.
	nonMatchingClause := ldbuilders.Clause("key", ldmodel.OperatorIn, ldvalue.String("not-"+context.Key()))
	inFallthrough := func(rollout ldmodel.Rollout) func(*ldbuilders.FlagBuilder) {
		return func(f *ldbuilders.FlagBuilder) {
			f.Fallthrough(ldmodel.VariationOrRollout{Rollout: rollout})
		}
	}
	trackedScenarios := []struct {
		name           string
		flagConfig     func(*ldbuilders.FlagBuilder)
		expectedReason string // empty means no full feature event is expected
	}{
		{
			name: "rule with trackEvents",
			flagConfig: func(f *ldbuilders.FlagBuilder) {
				f.AddRule(ldbuilders.NewRuleBuilder().ID("rule1").Variation(expectedVariation).TrackEvents(true))
			},
			expectedReason: `{"kind": "RULE_MATCH", "ruleIndex": 0, "ruleId": "rule1"}`,
		},
		{
			name: "fallthrough with trackEventsFallthrough",
			flagConfig: func(f *ldbuilders.FlagBuilder) {
				f.FallthroughVariation(expectedVariation).TrackEventsFallthrough(true)
			},
			expectedReason: `{"kind": "FALLTHROUGH"}`,
		},
		{
			name: "fallthrough when only a non-matching rule has trackEvents",
			flagConfig: func(f *ldbuilders.FlagBuilder) {
				f.AddRule(ldbuilders.NewRuleBuilder().ID("rule1").Variation(0).TrackEvents(true).
					Clauses(nonMatchingClause))
				f.FallthroughVariation(expectedVariation)
			},
		},
		{
			name: "rule match when only the fallthrough is tracked",
			flagConfig: func(f *ldbuilders.FlagBuilder) {
				f.AddRule(ldbuilders.NewRuleBuilder().ID("rule1").Variation(expectedVariation))
				f.FallthroughVariation(0).TrackEventsFallthrough(true)
			},
		},
		{
			name: "experiment in fallthrough with context in untracked bucket",
			flagConfig: inFallthrough(ldmodel.Rollout{
				Kind: ldmodel.RolloutKindExperiment,
				Variations: []ldmodel.WeightedVariation{
					{Variation: expectedVariation, Weight: 100000, Untracked: true},
				},
			}),
		},
		{
			name: "experiment in fallthrough for a context kind the context does not have",
			flagConfig: inFallthrough(ldmodel.Rollout{
				Kind:        ldmodel.RolloutKindExperiment,
				ContextKind: "other-kind",
				Variations: []ldmodel.WeightedVariation{
					{Variation: expectedVariation, Weight: 100000},
				},
			}),
		},
	}

	for _, scenario := range trackedScenarios {
		t.Run(scenario.name, func(t *ldtest.T) {
			builder := ldbuilders.NewFlagBuilder("flag-key").Version(1).On(true).
				Variations(wrongValue, expectedValue)
			scenario.flagConfig(builder)
			flag := builder.Build()
			data := mockld.NewServerSDKDataBuilder().Flag(flag).Build()

			dataSource := NewSDKDataSource(t, data)
			eventSink := NewSDKEventSink(t)
			client := NewSDKClient(t, dataSource, eventSink)

			result := basicEvaluateFlag(t, client, flag.Key, context, defaultValue)
			m.In(t).Assert(result, m.JSONEqual(expectedValue))

			client.FlushEvents(t)
			payload := eventSink.ExpectAnalyticsEvents(t, time.Second)

			var eventMatchers []m.Matcher
			if scenario.expectedReason != "" || isPHP {
				// The PHP SDK sends a feature event for every evaluation; it includes a reason only if
				// the evaluation is tracked.
				reasonMatcher := JSONPropertyNullOrAbsent("reason")
				if scenario.expectedReason != "" {
					reasonMatcher = m.JSONProperty("reason").Should(m.JSONStrEqual(scenario.expectedReason))
				}
				eventMatchers = append(eventMatchers, IsValidFeatureEventWithConditions(
					t, isPHP, context,
					m.JSONProperty("key").Should(m.Equal(flag.Key)),
					m.JSONProperty("version").Should(m.Equal(flag.Version)),
					m.JSONProperty("value").Should(m.JSONEqual(expectedValue)),
					m.JSONProperty("variation").Should(m.Equal(expectedVariation)),
					reasonMatcher,
					m.JSONProperty("default").Should(m.JSONEqual(defaultValue)),
					JSONPropertyNullOrAbsent("prereqOf"),
				))
			}
			if !isPHP {
				eventMatchers = append(eventMatchers, IsIndexEventForContext(context), IsSummaryEvent())
			}
			m.In(t).Assert(payload, m.ItemsInAnyOrder(eventMatchers...))
		})
	}
}
