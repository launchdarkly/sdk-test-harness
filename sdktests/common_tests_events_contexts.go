package sdktests

import (
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

type eventContextTestParams struct {
	name             string
	eventsConfig     servicedef.SDKConfigEventParams
	contextFactory   func(string) *data.ContextFactory
	outputContext    func(ldcontext.Context) ldcontext.Context
	redactedShouldBe []string
}

func makeEventContextTestParams() []eventContextTestParams {
	ret := []eventContextTestParams{
		// Note that in the output matchers, we can't just check for JSON equality with an entire
		// object, because 1. unique keys are generated for each test (the test logic will check
		// the keys separately) and 2. the redactedAttributes list can be in any order. So we have
		// to use nested matchers.
		{
			name: "single-kind minimal",
			contextFactory: func(prefix string) *data.ContextFactory {
				return data.NewContextFactory(prefix)
			},
		},
		{
			name: "multi-kind minimal",
			contextFactory: func(prefix string) *data.ContextFactory {
				return data.NewMultiContextFactory(prefix, []ldcontext.Kind{"org", "other"})
			},
		},
		{
			name: "single-kind with attributes, nothing private",
			// includes all built-in attributes plus a custom one, just to make sure they are copied
			contextFactory: func(prefix string) *data.ContextFactory {
				return data.NewContextFactory(prefix, func(b *ldcontext.Builder) {
					b.Kind("org")
					b.Name("a")
					b.SetString("b", "c")
					b.Anonymous(true)
				})
			},
		},
		{
			name: "single-kind, allAttributesPrivate",
			// proves that name and custom attributes are redacted, key/anonymous/meta are not
			eventsConfig: servicedef.SDKConfigEventParams{AllAttributesPrivate: true},
			contextFactory: func(prefix string) *data.ContextFactory {
				return data.NewContextFactory(prefix, func(b *ldcontext.Builder) {
					b.Name("a")
					b.SetString("b", "c")
					b.Anonymous(true)
				})
			},
			outputContext: func(c ldcontext.Context) ldcontext.Context {
				return ldcontext.NewBuilderFromContext(c).
					SetValue("name", ldvalue.Null()).
					SetValue("b", ldvalue.Null()).
					Build()
			},
			redactedShouldBe: []string{"name", "b"},
		},
		{
			name: "single-kind, specific private attributes",
			// here, "name" is declared private globally, and "b" is private per-context
			eventsConfig: servicedef.SDKConfigEventParams{
				GlobalPrivateAttributes: []string{"name"},
			},
			contextFactory: func(prefix string) *data.ContextFactory {
				return data.NewContextFactory(prefix, func(b *ldcontext.Builder) {
					b.Name("a")
					b.SetString("b", "c")
					b.SetString("d", "e")
					b.Private("b")
				})
			},
			outputContext: func(c ldcontext.Context) ldcontext.Context {
				return ldcontext.NewBuilderFromContext(c).
					SetValue("name", ldvalue.Null()).
					SetValue("b", ldvalue.Null()).
					Build()
			},
			redactedShouldBe: []string{"name", "b"},
		},
		{
			name: "single-kind, private attribute nested property",
			// redacting just part of an object value
			eventsConfig: servicedef.SDKConfigEventParams{
				GlobalPrivateAttributes: []string{"/c/prop2/sub1"},
			},
			contextFactory: func(prefix string) *data.ContextFactory {
				return data.NewContextFactory(prefix, func(b *ldcontext.Builder) {
					b.Name("a")
					b.SetValue("b", ldvalue.Parse([]byte(`{"prop1": true, "prop2": 3}`)))
					b.SetValue("c", ldvalue.Parse([]byte(`{"prop1": {"sub1": true}, "prop2": {"sub1": 4, "sub2": 5}}`)))
					b.Private("/b/prop1")
				})
			},
			outputContext: func(c ldcontext.Context) ldcontext.Context {
				return ldcontext.NewBuilderFromContext(c).
					SetValue("b", ldvalue.Parse([]byte(`{"prop2": 3}`))).
					SetValue("c", ldvalue.Parse([]byte(`{"prop1": {"sub1": true}, "prop2": {"sub2": 5}}`))).
					Build()
			},
			redactedShouldBe: []string{"/b/prop1", "/c/prop2/sub1"},
		},
	}
	// Add some test cases to verify that all possible value types can be used for a
	// custom attribute.
	for _, value := range data.MakeStandardTestValues() {
		if value.IsNull() {
			continue // custom attribute with null value would be dropped
		}
		value := value // due to contextFactory closure below
		ret = append(ret, eventContextTestParams{
			name: fmt.Sprintf("custom attribute with value %s", value.JSONString()),
			contextFactory: func(prefix string) *data.ContextFactory {
				return data.NewContextFactory(prefix, func(b *ldcontext.Builder) {
					b.SetValue("attr", value)
				})
			},
		})
	}
	return ret
}

func (c CommonEventTests) EventContexts(t *ldtest.T) {
	// Flags to use for "feature" and "debug" event tests
	// The flag variation/value is irrelevant.
	var flagKey, debuggedFlagKey string
	flagValue := ldvalue.String("value")
	var sdkData mockld.SDKData
	if c.isClientSide {
		flagKey, debuggedFlagKey = "flag", "debugged-flag"
		sdkData = mockld.NewClientSDKDataBuilder().
			Flag(flagKey, mockld.ClientSDKFlag{
				Value:     flagValue,
				Variation: o.Some(0),
			}).
			Flag(debuggedFlagKey, mockld.ClientSDKFlag{
				Value:                flagValue,
				Variation:            o.Some(0),
				DebugEventsUntilDate: o.Some(ldtime.UnixMillisNow() + 1000000),
			}).
			Build()
	} else {
		flags := data.NewFlagFactory("EventContexts", data.SingleValueForAllSDKValueTypes(flagValue))
		debugFlags := data.NewFlagFactory(
			"EventContextDebugFlag",
			data.SingleValueForAllSDKValueTypes(flagValue),
			data.FlagShouldAlwaysHaveDebuggingEnabled,
		)
		flag, debugFlag := flags.MakeFlag(), debugFlags.MakeFlag()
		flagKey, debuggedFlagKey = flag.Key, debugFlag.Key
		sdkData = mockld.NewServerSDKDataBuilder().Flag(flag, debugFlag).Build()
	}

	dataSource := NewSDKDataSource(t, sdkData)

	for _, p := range makeEventContextTestParams() {
		outputMatcher := func(context ldcontext.Context) m.Matcher {
			expectedContext := context
			if p.outputContext != nil {
				expectedContext = p.outputContext(context)
			}
			return JSONMatchesEventContext(expectedContext, p.redactedShouldBe)
		}

		identifyEventForContext := func(context ldcontext.Context) m.Matcher {
			return m.AllOf(
				IsIdentifyEventForContext(context),
				m.JSONProperty("context").Should(outputMatcher(context)),
			)
		}

		contexts := p.contextFactory("doServerSideEventContextTests")

		t.Run(p.name, func(t *ldtest.T) {
			events := NewSDKEventSink(t)

			initialContext := contexts.NextUniqueContext()
			client := NewSDKClient(t, c.baseSDKConfigurationPlus(
				WithClientSideInitialContext(initialContext),
				WithEventsConfig(p.eventsConfig),
				dataSource,
				events)...)

			if c.isClientSide {
				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				t.Run("initial identify event", func(t *ldtest.T) {
					m.In(t).Assert(payload, m.Items(identifyEventForContext(initialContext)))
				})
			}

			if c.isPHP { // only the PHP SDK sends inline contexts in feature events
				t.Run("feature event", func(t *ldtest.T) {
					defaultValue := ldvalue.String("default")
					context := contexts.NextUniqueContext()
					verifyResult := func(t *ldtest.T) {
						client.FlushEvents(t)
						payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
						eventMatchers := []m.Matcher{
							m.AllOf(
								IsValidFeatureEventWithConditions(true, context,
									m.JSONProperty("context").Should(outputMatcher(context))),
							),
						}
						m.In(t).Assert(payload, m.Items(eventMatchers...))
					}

					_ = basicEvaluateFlag(t, client, flagKey, context, defaultValue)
					verifyResult(t)

					if user := representContextAsOldUser(t, context); user != nil {
						t.Run("with old user", func(t *ldtest.T) {
							_ = basicEvaluateFlagWithOldUser(t, client, flagKey, user, defaultValue)
							verifyResult(t)
						})
					}
				})
			}

			if !c.isPHP { // PHP SDK does not send debug events - it just passes along the debugEventsUntilDate property
				t.Run("debug event", func(t *ldtest.T) {
					defaultValue := ldvalue.String("default")
					context := contexts.NextUniqueContext()
					if c.isClientSide {
						client.SendIdentifyEvent(t, context)
					}
					_ = basicEvaluateFlag(t, client, debuggedFlagKey, context, ldvalue.String("default"))
					client.FlushEvents(t)

					payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

					debugEventMatcher := m.AllOf(
						IsDebugEvent(),
						m.JSONProperty("context").Should(outputMatcher(context)),
					)
					eventMatchers := []m.Matcher{debugEventMatcher, IsSummaryEvent()}
					if c.isClientSide {
						eventMatchers = append(eventMatchers, IsIdentifyEvent())
					} else {
						eventMatchers = append(eventMatchers, IsIndexEvent())
					}
					m.In(t).Assert(payload, m.ItemsInAnyOrder(eventMatchers...))

					if user := representContextAsOldUser(t, context); user != nil {
						t.Run("with old user", func(t *ldtest.T) {
							if c.isClientSide {
								client.SendIdentifyEventWithOldUser(t, user)
							}
							_ = basicEvaluateFlagWithOldUser(t, client, debuggedFlagKey, user, defaultValue)
							client.FlushEvents(t)
							payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
							eventMatchers := []m.Matcher{debugEventMatcher, IsSummaryEvent()}
							if c.isClientSide {
								eventMatchers = append(eventMatchers, IsIdentifyEvent())
							} // do _not_ expect an index event from server-side SDKs here, because this context key has already been seen
							m.In(t).Assert(payload, m.ItemsInAnyOrder(eventMatchers...))
						})
					}
				})
			}

			t.Run("identify event", func(t *ldtest.T) {
				context := contexts.NextUniqueContext()

				verifyResult := func(t *ldtest.T) {
					client.FlushEvents(t)
					payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
					m.In(t).Assert(payload, m.Items(
						m.AllOf(
							IsIdentifyEvent(),
							HasContextObjectWithMatchingKeys(context),
							m.JSONProperty("context").Should(outputMatcher(context)),
						),
					))
				}

				client.SendIdentifyEvent(t, context)
				verifyResult(t)

				if user := representContextAsOldUser(t, context); user != nil {
					t.Run("with old user", func(t *ldtest.T) {
						client.SendIdentifyEventWithOldUser(t, user)
						verifyResult(t)
					})
				}
			})

			if !c.isClientSide && !c.isPHP { // client-side SDKs and the PHP SDK never send index events
				expectedIndexEvent := func(c ldcontext.Context) m.Matcher {
					return m.AllOf(
						IsIndexEvent(),
						HasContextObjectWithMatchingKeys(c),
						m.JSONProperty("context").Should(outputMatcher(c)),
					)
				}

				t.Run("index event from evaluation", func(t *ldtest.T) {
					// Doing an evaluation for a never-before-seen user will generate an index event. We don't
					// care about the evaluation result or the summary data, we're just looking at the user
					// properties in the index event itself.
					verifyResult := func(t *ldtest.T, c ldcontext.Context) {
						client.FlushEvents(t)
						payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
						m.In(t).Assert(payload, m.ItemsInAnyOrder(expectedIndexEvent(c), IsSummaryEvent()))
					}

					context := contexts.NextUniqueContext()
					basicEvaluateFlag(t, client, "arbitrary-flag-key", context, ldvalue.Null())
					verifyResult(t, context)

					// Before we try converting the context to a user, we need to make sure it has a different key
					// since the index event test depends on it being a never-before-seen key
					context2 := contextWithTransformedKeys(context, func(key string) string { return key + ".olduser" })
					if user := representContextAsOldUser(t, context2); user != nil {
						t.Run("with old user", func(t *ldtest.T) {
							basicEvaluateFlagWithOldUser(t, client, "arbitrary-flag-key", user, ldvalue.Null())
							verifyResult(t, context2)
						})
					}
				})

				t.Run("index event from custom event", func(t *ldtest.T) {
					// Sending a custom event for a never-before-seen user will generate an index event.
					verifyResult := func(t *ldtest.T, c ldcontext.Context) {
						client.FlushEvents(t)
						payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
						m.In(t).Assert(payload, m.ItemsInAnyOrder(expectedIndexEvent(c), IsCustomEvent()))
					}

					context := contexts.NextUniqueContext()
					client.SendCustomEvent(t, servicedef.CustomEventParams{EventKey: "event-key", Context: o.Some(context)})
					verifyResult(t, context)

					context2 := contextWithTransformedKeys(context, func(key string) string { return key + ".olduser" })
					if user := representContextAsOldUser(t, context2); user != nil {
						t.Run("with old user", func(t *ldtest.T) {
							client.SendCustomEvent(t, servicedef.CustomEventParams{EventKey: "event-key", User: user})
							verifyResult(t, context2)
						})
					}
				})
			}
		})

		if c.isClientSide {
			initialContext2 := contexts.NextUniqueContext()
			if user := representContextAsOldUser(t, initialContext2); user != nil {
				t.Run(p.name+" initial identify event with old user", func(t *ldtest.T) {
					events := NewSDKEventSink(t)

					client := NewSDKClient(t, c.baseSDKConfigurationPlus(
						WithClientSideConfig(servicedef.SDKConfigClientSideParams{InitialUser: user}),
						WithEventsConfig(p.eventsConfig),
						dataSource,
						events)...)

					client.FlushEvents(t)
					payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
					m.In(t).Assert(payload, m.Items(identifyEventForContext(initialContext2)))
				})
			}
		}
	}
}
