package sdktests

import (
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

type eventContextTestParams struct {
	name           string
	eventsConfig   servicedef.SDKConfigEventParams
	contextFactory func(string) *data.ContextFactory
	outputMatcher  m.Matcher
}

func makeEventContextTestParams() []eventContextTestParams {
	anyKeyMatcher := m.KV("key", m.Not(m.BeNil()))
	defaultKindMatcher := m.KV("kind", m.Equal(string(ldcontext.DefaultKind)))

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
			outputMatcher: m.MapOf(anyKeyMatcher, defaultKindMatcher),
		},
		{
			name: "multi-kind minimal",
			contextFactory: func(prefix string) *data.ContextFactory {
				return data.NewMultiContextFactory(prefix, []ldcontext.Kind{"org", "other"})
			},
			outputMatcher: m.MapOf(
				m.KV("kind", m.Equal("multi")),
				m.KV("org", m.MapOf(anyKeyMatcher)),
				m.KV("other", m.MapOf(anyKeyMatcher)),
			),
		},
		{
			name: "single-kind with attributes, nothing private",
			// includes all built-in attributes plus a custom one, just to make sure they are copied
			contextFactory: func(prefix string) *data.ContextFactory {
				return data.NewContextFactory(prefix, func(b *ldcontext.Builder) {
					b.Kind("org")
					b.Name("a")
					b.SetString("b", "c")
					b.Secondary("s")
					b.Transient(true)
				})
			},
			outputMatcher: m.MapOf(
				anyKeyMatcher,
				m.KV("kind", m.Equal("org")),
				m.KV("name", m.Equal("a")),
				m.KV("b", m.Equal("c")),
				m.KV("transient", m.Equal(true)),
				m.KV("_meta", m.MapOf(
					m.KV("secondary", m.Equal("s")),
				)),
			),
		},
		{
			name: "single-kind, allAttributesPrivate",
			// proves that name and custom attributes are redacted, key/transient/meta are not
			eventsConfig: servicedef.SDKConfigEventParams{AllAttributesPrivate: true},
			contextFactory: func(prefix string) *data.ContextFactory {
				return data.NewContextFactory(prefix, func(b *ldcontext.Builder) {
					b.Name("a")
					b.SetString("b", "c")
					b.Secondary("s")
					b.Transient(true)
				})
			},
			outputMatcher: m.MapOf(
				anyKeyMatcher, defaultKindMatcher,
				m.KV("transient", m.Equal(true)),
				m.KV("_meta", m.MapOf(
					m.KV("secondary", m.Equal("s")),
					m.KV("redactedAttributes", RedactedAttributesAre("name", "b")),
				)),
			),
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
			outputMatcher: m.MapOf(
				anyKeyMatcher, defaultKindMatcher,
				m.KV("d", m.Equal("e")),
				m.KV("_meta", m.MapOf(
					m.KV("redactedAttributes", RedactedAttributesAre("name", "b")),
				)),
			),
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
			outputMatcher: m.MapOf(
				anyKeyMatcher, defaultKindMatcher,
				m.KV("name", m.Equal("a")),
				m.KV("b", m.JSONStrEqual(`{"prop2": 3}`)),
				m.KV("c", m.JSONStrEqual(`{"prop1": {"sub1": true}, "prop2": {"sub2": 5}}`)),
				m.KV("_meta", m.MapOf(
					m.KV("redactedAttributes", RedactedAttributesAre("/b/prop1", "/c/prop2/sub1")),
				)),
			),
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
			outputMatcher: m.MapOf(
				anyKeyMatcher, defaultKindMatcher,
				m.KV("attr", m.JSONEqual(value)),
			),
		})
	}
	return ret
}

func doServerSideEventContextTests(t *ldtest.T) {
	flagValue := ldvalue.String("value")
	defaultValue := ldvalue.String("default")
	flags := data.NewFlagFactory(
		"ServerSideEvalEventContextFlag",
		data.SingleValueForAllSDKValueTypes(flagValue),
	)
	debugFlags := data.NewFlagFactory(
		"ServerSideEvalEventContextDebugFlag",
		data.SingleValueForAllSDKValueTypes(flagValue),
		data.FlagShouldAlwaysHaveDebuggingEnabled,
	)
	flag := flags.MakeFlag()
	debugFlag := debugFlags.MakeFlag()
	dataSource := NewSDKDataSource(t, mockld.NewServerSDKDataBuilder().Flag(flag, debugFlag).Build())
	events := NewSDKEventSink(t)

	for _, p := range makeEventContextTestParams() {
		t.Run(p.name, func(t *ldtest.T) {
			contexts := p.contextFactory("doServerSideEventContextTests")
			client := NewSDKClient(t, WithEventsConfig(p.eventsConfig), dataSource, events)

			t.Run("debug event", func(t *ldtest.T) {
				context := contexts.NextUniqueContext()
				client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
					FlagKey:      debugFlag.Key,
					Context:      context,
					ValueType:    servicedef.ValueTypeAny,
					DefaultValue: defaultValue,
				})
				client.FlushEvents(t)

				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.ItemsInAnyOrder(
					IsIndexEvent(),
					m.AllOf(
						IsDebugEvent(),
						HasContextObjectWithMatchingKeys(context),
						m.JSONProperty("context").Should(p.outputMatcher),
					),
					IsSummaryEvent(),
				))
			})

			t.Run("identify event", func(t *ldtest.T) {
				context := contexts.NextUniqueContext()
				client.SendIdentifyEvent(t, context)
				client.FlushEvents(t)

				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.Items(
					m.AllOf(
						IsIdentifyEvent(),
						HasContextObjectWithMatchingKeys(context),
						m.JSONProperty("context").Should(p.outputMatcher),
					),
				))
			})

			t.Run("index event", func(t *ldtest.T) {
				context := contexts.NextUniqueContext()
				basicEvaluateFlag(t, client, "arbitrary-flag-key", context, ldvalue.Null())
				client.FlushEvents(t)

				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.ItemsInAnyOrder(
					m.AllOf(
						IsIndexEvent(),
						HasContextObjectWithMatchingKeys(context),
						m.JSONProperty("context").Should(p.outputMatcher),
					),
					IsSummaryEvent(),
				))
			})
		})
	}
}
