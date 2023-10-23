package sdktests

import (
	"fmt"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/data"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/require"
)

// The PHP's analytics event behavior regarding evaluations is different enough from the other
// server-side SDKs that it would be hard to share the test implementation without having a
// confusing amount of conditional behavior.
//
// - Every evaluation produces a "feature" event.
// - There are never any "index" or "summary" events.
// - The event always contains an inline user.
// - The "trackEvents" and "debugEventsUntilDate" properties of the flag do not affect what
//   kind of events are produced. Instead, the PHP SDK simply includes those properties in the
//   event-- to be used when the Relay Proxy summarizes the event data.

func doPHPFeatureEventTests(t *ldtest.T) {
	valueFactories := data.MakeValueFactoriesBySDKValueType(2)
	flagValues, defaultValues := valueFactories[0], valueFactories[1]
	contextFactories := data.NewContextFactoriesForSingleAndMultiKind("doPHPFeatureEventTests")
	expectedReason := ldreason.NewEvalReasonFallthrough()
	debugDate := ldtime.UnixMillisecondTime(12345678)

	type flagSelectors struct {
		tracked, withDebug, malformed bool
	}
	describe := func(fs flagSelectors) string {
		return fmt.Sprintf("%s-%s-%s",
			h.IfElse(fs.tracked, "tracked", "untracked"),
			h.IfElse(fs.withDebug, "debug", "nodebug"),
			h.IfElse(fs.malformed, "malformed", "valid"))
	}
	var allFlagSelectors []flagSelectors
	for _, tracked := range []bool{false, true} {
		for _, withDebug := range []bool{false, true} {
			for _, malformed := range []bool{false, true} {
				allFlagSelectors = append(allFlagSelectors, flagSelectors{tracked, withDebug, malformed})
			}
		}
	}
	flagFactories := make(map[flagSelectors]*data.FlagFactory)
	for _, fs := range allFlagSelectors {
		fs := fs
		flagFactories[fs] = data.NewFlagFactory(
			fmt.Sprintf("%s-flag", describe(fs)),
			flagValues,
			data.FlagShouldProduceThisEvalReason(expectedReason),
			func(b *ldbuilders.FlagBuilder) {
				b.TrackEvents(fs.tracked)
				if fs.withDebug {
					b.DebugEventsUntilDate(debugDate)
				}
				if fs.malformed {
					b.On(false).OffVariation(-1)
				}
			},
		)
	}

	dataBuilder := mockld.NewServerSDKDataBuilder()
	for _, factory := range flagFactories {
		for _, valueType := range getValueTypesToTest(t) {
			dataBuilder.Flag(factory.MakeFlagForValueType(valueType))
		}
	}
	excludeFromSummaries := ldbuilders.NewFlagBuilder("exclude-from-summaries").Version(1).
		On(true).Variations(ldvalue.Bool(true), ldvalue.Bool(false)).ExcludeFromSummaries(true).Build()
	dataBuilder.Flag(excludeFromSummaries)

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSink(t)

	client := NewSDKClient(t, dataSource, events)

	doFeatureEventTests := func(t *ldtest.T, fs flagSelectors, withReason bool) {
		for _, contextFactory := range contextFactories {
			t.Run(contextFactory.Description(), func(t *ldtest.T) {
				for _, valueType := range getValueTypesToTest(t) {
					t.Run(testDescFromType(valueType), func(t *ldtest.T) {
						flag := flagFactories[fs].ReuseFlagForValueType(valueType)
						var expectedValue ldvalue.Value
						var expectedVariation o.Maybe[int]
						if fs.malformed {
							expectedValue = defaultValues(valueType)
							expectedVariation = o.None[int]()
						} else {
							expectedValue = flagValues(valueType)
							expectedVariation = o.Some(0)
						}
						context := contextFactory.NextUniqueContext()
						resp := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
							FlagKey:      flag.Key,
							Context:      o.Some(context),
							ValueType:    valueType,
							DefaultValue: defaultValues(valueType),
							Detail:       withReason,
						})

						// If the evaluation didn't return the expected value, then the rest of the test is moot
						if !m.In(t).Assert(resp.Value, m.JSONEqual(expectedValue)) {
							require.Fail(t, "evaluation unexpectedly returned wrong value")
						}

						client.FlushEvents(t)

						reason := expectedReason
						if fs.malformed {
							reason = ldreason.NewEvalReasonError(ldreason.EvalErrorMalformedFlag)
						}
						propMatchers := []m.Matcher{
							m.JSONProperty("key").Should(m.Equal(flag.Key)),
							m.JSONProperty("version").Should(m.Equal(flag.Version)),
							m.JSONProperty("value").Should(m.JSONEqual(expectedValue)),
							m.JSONOptProperty("variation").Should(m.JSONEqual(expectedVariation)),
							maybeReason(withReason, reason),
							m.JSONProperty("default").Should(m.JSONEqual(defaultValues(valueType))),
							JSONPropertyNullOrAbsent("prereqOf"),
							m.JSONOptProperty("trackEvents").Should(
								h.IfElse(fs.tracked, m.JSONEqual(true), m.AnyOf(m.BeNil(), m.JSONEqual(false))),
							),
							m.JSONOptProperty("debugEventsUntilDate").Should(
								h.IfElse(fs.withDebug, m.JSONEqual(debugDate), m.BeNil()),
							),
						}
						matchFeatureEvent := IsValidFeatureEventWithConditions(true, context, propMatchers...)

						payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
						m.In(t).Assert(payload, m.Items(matchFeatureEvent))
					})
				}
			})
		}
	}

	for _, fs := range allFlagSelectors {
		t.Run(describe(fs), func(t *ldtest.T) {
			for _, withReason := range []bool{false, true} {
				t.Run(h.IfElse(withReason, "with reason", "without reason"), func(t *ldtest.T) {
					doFeatureEventTests(t, fs, withReason)
				})
			}
		})
	}

	t.Run("evaluating all flags generates no events", func(t *ldtest.T) {
		_ = client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
			Context: o.Some(contextFactories[0].NextUniqueContext()),
		})
		client.FlushEvents(t)
		events.ExpectNoAnalyticsEvents(t, time.Millisecond*200)
	})

	if t.Capabilities().Has(servicedef.CapabilityEventSampling) {
		t.Run("exclude from summaries is set correctly", func(t *ldtest.T) {
			context := ldcontext.New("user-key")
			client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
				FlagKey:      excludeFromSummaries.Key,
				Context:      o.Some(context),
				ValueType:    servicedef.ValueTypeBool,
				DefaultValue: ldvalue.Bool(false),
				Detail:       false,
			})

			client.FlushEvents(t)

			propMatchers := []m.Matcher{m.JSONProperty("excludeFromSummaries").Should(m.Equal(true))}
			matchFeatureEvent := IsValidFeatureEventWithConditions(true, context, propMatchers...)

			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
			m.In(t).Assert(payload, m.Items(matchFeatureEvent))
		})
	}
}
