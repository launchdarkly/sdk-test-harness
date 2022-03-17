package sdktests

import (
	"fmt"
	"sort"
	"strings"

	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldcontext"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldvalue"
)

type eventUserTestScenario struct {
	config           servicedef.SDKConfigEventParams
	userPrivateAttrs []string
}

func (s eventUserTestScenario) MakeUser(originalUser ldcontext.Context) ldcontext.Context {
	if len(s.userPrivateAttrs) == 0 {
		return originalUser
	}
	builder := ldcontext.NewBuilderFromContext(originalUser)
	for _, a := range s.userPrivateAttrs {
		builder.Private(a)
	}
	return builder.Build()
}

func (s eventUserTestScenario) hasExpectedUserObject(user ldcontext.Context) m.Matcher {
	return m.JSONProperty("context").Should(eventUserMatcher(user, s.config))
}

func (s eventUserTestScenario) Description() string {
	var parts []string
	if s.config.AllAttributesPrivate {
		parts = append(parts, "allAttributesPrivate=true")
	}
	if len(s.config.GlobalPrivateAttributes) != 0 {
		parts = append(parts, fmt.Sprintf("globally-private=%v", s.config.GlobalPrivateAttributes))
	}
	if len(s.userPrivateAttrs) != 0 {
		parts = append(parts, fmt.Sprintf("user-private=%v", s.userPrivateAttrs))
	}
	if len(parts) == 0 {
		parts = append(parts, "no-attributes-filtered")
	}

	return strings.Join(parts, ", ")
}

func doServerSideEventUserTests(t *ldtest.T) {
	var scenarios []eventUserTestScenario
	for _, allAttrsPrivate := range []bool{false, true} {
		for _, globalPrivateAttrs := range [][]string{nil, {"firstName"}} {
			for _, userPrivateAttrs := range [][]string{nil, {"lastName", "preferredLanguage"}} {
				scenarios = append(scenarios, eventUserTestScenario{
					config: servicedef.SDKConfigEventParams{
						AllAttributesPrivate:    allAttrsPrivate,
						GlobalPrivateAttributes: globalPrivateAttrs,
					},
					userPrivateAttrs: userPrivateAttrs,
				})
			}
		}
	}

	flagValue := ldvalue.String("value")
	defaultValue := ldvalue.String("default")
	contexts := data.NewContextFactory(
		"doServerSideEventUserTests",
		func(b *ldcontext.Builder) {
			b.SetString("firstName", "first").SetString("lastName", "last").SetString("country", "us").
				SetString("preferredLanguage", "go").SetString("primaryLanguage", "go")
		})
	flags := data.NewFlagFactory(
		"ServerSideEvalEventUserFlag",
		data.SingleValueForAllSDKValueTypes(flagValue),
		data.FlagShouldHaveFullEventTracking,
	)
	flag := flags.MakeFlag()
	dataSource := NewSDKDataSource(t, mockld.NewServerSDKDataBuilder().Flag(flag).Build())
	events := NewSDKEventSink(t)

	for _, scenario := range scenarios {
		t.Run(scenario.Description(), func(t *ldtest.T) {
			client := NewSDKClient(t, WithEventsConfig(scenario.config), dataSource, events)

			t.Run("feature event", func(t *ldtest.T) {
				user := scenario.MakeUser(contexts.NextUniqueContext())
				client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
					FlagKey:      flag.Key,
					Context:      user,
					ValueType:    servicedef.ValueTypeAny,
					DefaultValue: defaultValue,
				})
				client.FlushEvents(t)

				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.ItemsInAnyOrder(
					IsIndexEvent(),
					m.AllOf(IsFeatureEvent(), HasNoUserObject(), HasContextKeys(user)),
					IsSummaryEvent(),
				))
			})

			t.Run("identify event", func(t *ldtest.T) {
				user := scenario.MakeUser(contexts.NextUniqueContext())
				client.SendIdentifyEvent(t, user)
				client.FlushEvents(t)

				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.Items(
					m.AllOf(IsIdentifyEvent(), scenario.hasExpectedUserObject(user)),
				))
			})

			t.Run("custom event", func(t *ldtest.T) {
				user := scenario.MakeUser(contexts.NextUniqueContext())
				eventData := ldvalue.Bool(true)
				metricValue := float64(10)
				client.SendCustomEvent(t, servicedef.CustomEventParams{
					Context:     user,
					EventKey:    "event-key",
					Data:        eventData,
					MetricValue: &metricValue,
				})
				client.FlushEvents(t)

				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.ItemsInAnyOrder(
					m.AllOf(IsIndexEvent()),
					m.AllOf(IsCustomEvent(), HasNoUserObject(), HasContextKeys(user)),
				))
			})

			t.Run("index event", func(t *ldtest.T) {
				user := scenario.MakeUser(contexts.NextUniqueContext())
				basicEvaluateFlag(t, client, "arbitrary-flag-key", user, ldvalue.Null())
				client.FlushEvents(t)

				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.ItemsInAnyOrder(
					m.AllOf(IsIndexEvent(), scenario.hasExpectedUserObject(user)),
					IsSummaryEvent(),
				))
			})
		})
	}
}

// eventUserMatcher returns a Matcher to verify that a JSON object has the expected properties based on
// the input context and the events configuration. This mostly means verifying that private attributes
// behave correctly.
//
// Because the rules for private attributes are fairly complicated, we have not reimplemented that logic
// here. Instead, this function depends on the Go SDK's implementation of event context formatting,
// treating it as a reference implementation that has been validated by its own thorough tests.
func eventUserMatcher(context ldcontext.Context, eventsConfig servicedef.SDKConfigEventParams) m.Matcher {
	kvs := []m.KeyValueMatcher{
		m.KV("kind", m.Equal(string(context.Kind()))),
		m.KV("key", m.Equal(context.Key())),
	}
	if context.Transient() {
		kvs = append(kvs, m.KV("transient", m.Equal(true)))
	}
	isPrivate := func(name string) bool {
		if eventsConfig.AllAttributesPrivate {
			return true
		}
		for _, a := range eventsConfig.GlobalPrivateAttributes {
			if name == a {
				return true
			}
		}
		for i := 0; i < context.PrivateAttributeCount(); i++ {
			if a, _ := context.PrivateAttributeByIndex(i); name == a.String() {
				return true
			}
		}
		return false
	}
	redacted := []string{}
	for _, name := range context.GetOptionalAttributeNames(nil) {
		if value, ok := context.GetValue(name); ok {
			if isPrivate(name) {
				redacted = append(redacted, name)
			} else {
				kvs = append(kvs, m.KV(name, m.JSONEqual(value)))
			}
		}
	}
	if len(redacted) != 0 || context.Secondary().IsDefined() {
		kvsMeta := []m.KeyValueMatcher{}
		if context.Secondary().IsDefined() {
			kvsMeta = append(kvsMeta, m.KV("secondary", m.Equal(context.Secondary().StringValue())))
		}
		if len(redacted) != 0 {
			sort.Strings(redacted)
			kvsMeta = append(kvsMeta, m.KV("redactedAttributes", SortedStrings().Should(m.Equal(redacted))))
		}
		kvs = append(kvs, m.KV("_meta", m.MapOf(kvsMeta...)))
	}
	return m.JSONMap().Should(m.MapOf(kvs...))
}
