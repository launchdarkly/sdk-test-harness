package sdktests

import (
	"fmt"
	"time"

	h "github.com/launchdarkly/sdk-test-harness/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"

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
	flagValues := FlagValueByTypeFactory()
	defaultValues := DefaultValueByTypeFactory()
	users := NewUserFactory("doPHPFeatureEventTests")
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
	flagFactories := make(map[flagSelectors]*FlagFactoryForValueTypes)
	for _, fs := range allFlagSelectors {
		fs := fs
		flagFactories[fs] = &FlagFactoryForValueTypes{
			KeyPrefix: fmt.Sprintf("%s-flag", describe(fs)),
			Reason:    expectedReason,
			BuilderActions: func(b *ldbuilders.FlagBuilder) {
				b.TrackEvents(fs.tracked)
				if fs.withDebug {
					b.DebugEventsUntilDate(debugDate)
				}
				if fs.malformed {
					b.On(false).OffVariation(-1)
				}
			}}
	}

	dataBuilder := mockld.NewServerSDKDataBuilder()
	for _, factory := range flagFactories {
		for _, valueType := range getValueTypesToTest(t) {
			dataBuilder.Flag(factory.ForType(valueType))
		}
	}

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSink(t)

	client := NewSDKClient(t, dataSource, events)

	doFeatureEventTest := func(t *ldtest.T, fs flagSelectors, withReason, isAnonymousUser bool) {
		for _, valueType := range getValueTypesToTest(t) {
			t.Run(testDescFromType(valueType), func(t *ldtest.T) {
				flag := flagFactories[fs].ForType(valueType)
				var expectedValue ldvalue.Value
				var expectedVariation o.Maybe[int]
				if fs.malformed {
					expectedValue = defaultValues(valueType)
					expectedVariation = o.None[int]()
				} else {
					expectedValue = flagValues(valueType)
					expectedVariation = o.Some(0)
				}
				user := users.NextUniqueUserMaybeAnonymous(isAnonymousUser)
				resp := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
					FlagKey:      flag.Key,
					User:         o.Some(user),
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
				matchFeatureEvent := IsValidFeatureEventWithConditions(true, true, user, propMatchers...)

				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.Items(matchFeatureEvent))
			})
		}
	}

	for _, fs := range allFlagSelectors {
		t.Run(describe(fs), func(t *ldtest.T) {
			for _, withReason := range []bool{false, true} {
				t.Run(h.IfElse(withReason, "with reason", "without reason"), func(t *ldtest.T) {
					for _, isAnonymousUser := range []bool{false, true} {
						t.Run(h.IfElse(isAnonymousUser, "anonymous user", "non-anonymous user"), func(t *ldtest.T) {
							doFeatureEventTest(t, fs, withReason, isAnonymousUser)
						})
					}
				})
			}
		})
	}

	t.Run("evaluating all flags generates no events", func(t *ldtest.T) {
		_ = client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
			User: o.Some(users.NextUniqueUser()),
		})
		client.FlushEvents(t)
		events.ExpectNoAnalyticsEvents(t, time.Millisecond*200)
	})
}
