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
	debuggedFlagKey := "debugged-flag"
	data := c.makeSDKDataWithDebuggedFlag(debuggedFlagKey)
	dataSource := NewSDKDataSource(t, data)

	for _, p := range makeEventContextTestParams() {
		outputMatcher := func(context ldcontext.Context) m.Matcher {
			expectedContext := context
			if p.outputContext != nil {
				expectedContext = p.outputContext(context)
			}
			return JSONMatchesEventContext(expectedContext, p.redactedShouldBe)
		}

		t.Run(p.name, func(t *ldtest.T) {
			contexts := p.contextFactory("doServerSideEventContextTests")
			events := NewSDKEventSink(t)
			client := NewSDKClient(t, c.baseSDKConfigurationPlus(WithEventsConfig(p.eventsConfig), dataSource, events)...)

			c.discardIdentifyEventIfClientSide(t, client, events) // client-side SDKs always send an initial identify

			if !c.isPHP { // PHP SDK does not send debug events - it just passes along the debugEventsUntilDate property
				t.Run("debug event", func(t *ldtest.T) {
					context := contexts.NextUniqueContext()
					if c.isClientSide {
						client.SendIdentifyEvent(t, context)
					}
					client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
						FlagKey:      debuggedFlagKey,
						Context:      o.Some(context),
						ValueType:    servicedef.ValueTypeAny,
						DefaultValue: ldvalue.String("default"),
					})
					client.FlushEvents(t)

					payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

					eventMatchers := []m.Matcher{
						m.AllOf(
							IsDebugEvent(),
							HasContextObjectWithMatchingKeys(context),
							m.JSONProperty("context").Should(outputMatcher(context)),
						),
					}
					if c.isClientSide {
						eventMatchers = append(eventMatchers, IsIdentifyEvent())
					} else if !c.isPHP {
						eventMatchers = append(eventMatchers, IsIndexEvent(), IsSummaryEvent())
					}
					m.In(t).Assert(payload, m.ItemsInAnyOrder(eventMatchers...))
				})
			}

			t.Run("identify event", func(t *ldtest.T) {
				context := contexts.NextUniqueContext()
				client.SendIdentifyEvent(t, context)

				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.Items(
					m.AllOf(
						IsIdentifyEvent(),
						HasContextObjectWithMatchingKeys(context),
						m.JSONProperty("context").Should(outputMatcher(context)),
					),
				))
			})

			if !c.isClientSide && !c.isPHP { // client-side SDKs and the PHP SDK never send index events
				t.Run("index event", func(t *ldtest.T) {
					// Doing an evaluation for a never-before-seen user will generate an index event. We don't
					// care about the evaluation result or the summary data, we're just looking at the user
					// properties in the index event itself.
					context := contexts.NextUniqueContext()
					basicEvaluateFlag(t, client, "arbitrary-flag-key", context, ldvalue.Null())
					client.FlushEvents(t)

					payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
					m.In(t).Assert(payload, m.ItemsInAnyOrder(
						m.AllOf(
							IsIndexEvent(),
							HasContextObjectWithMatchingKeys(context),
							m.JSONProperty("context").Should(outputMatcher(context)),
						),
						IsSummaryEvent(),
					))
				})
			}
		})
	}
}

func (c CommonEventTests) makeSDKDataWithDebuggedFlag(debuggedFlagKey string) mockld.SDKData {
	// This sets up the SDK data so that evaluating this flags will produce a debug event.
	// The flag variation/value is irrelevant.
	flagValue := ldvalue.String("value")

	if c.isClientSide {
		return mockld.NewClientSDKDataBuilder().
			Flag(debuggedFlagKey, mockld.ClientSDKFlag{
				Value:                flagValue,
				Variation:            o.Some(0),
				DebugEventsUntilDate: o.Some(ldtime.UnixMillisNow() + 1000000),
			}).
			Build()
	}

	debugFlags := data.NewFlagFactory(
		"EventContextDebugFlag",
		data.SingleValueForAllSDKValueTypes(flagValue),
		data.FlagShouldAlwaysHaveDebuggingEnabled,
	)
	flag := debugFlags.MakeFlag()
	flag.Key = debuggedFlagKey
	return mockld.NewServerSDKDataBuilder().Flag(flag).Build()
}
