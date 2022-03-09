package sdktests

import (
	"fmt"
	"sort"
	"strings"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
)

type eventUserTestScenario struct {
	config           servicedef.SDKConfigEventParams
	userPrivateAttrs []string
}

func (s eventUserTestScenario) MakeUser(originalUser lduser.User) lduser.User {
	if len(s.userPrivateAttrs) == 0 {
		return originalUser
	}
	ub := lduser.NewUserBuilderFromUser(originalUser)
	for _, a := range s.userPrivateAttrs {
		ub.SetAttribute(lduser.UserAttribute(a), originalUser.GetAttribute(lduser.UserAttribute(a))).AsPrivateAttribute()
	}
	return ub.Build()
}

func (s eventUserTestScenario) hasExpectedUserObject(user lduser.User) m.Matcher {
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
	users := NewUserFactory("doServerSideEventUserTests",
		func(ub lduser.UserBuilder) {
			ub.FirstName("first").LastName("last").Country("us").
				Custom("preferredLanguage", ldvalue.String("go")).Custom("primaryLanguage", ldvalue.String("go"))
		})
	flags := FlagFactoryForValueTypes{
		KeyPrefix:      "ServerSideEvalEventUserFlag",
		ValueFactory:   SingleValueFactory(flagValue),
		BuilderActions: func(b *ldbuilders.FlagBuilder) { b.TrackEvents(true) },
	}
	flag := flags.ForType(servicedef.ValueTypeAny)
	dataSource := NewSDKDataSource(t, mockld.NewServerSDKDataBuilder().Flag(flag).Build())
	events := NewSDKEventSink(t)

	for _, scenario := range scenarios {
		t.Run(scenario.Description(), func(t *ldtest.T) {
			client := NewSDKClient(t, WithEventsConfig(scenario.config), dataSource, events)

			t.Run("feature event", func(t *ldtest.T) {
				user := scenario.MakeUser(users.NextUniqueUser())
				client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
					FlagKey:      flag.Key,
					User:         user,
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
				user := scenario.MakeUser(users.NextUniqueUser())
				client.SendIdentifyEvent(t, user)
				client.FlushEvents(t)

				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.Items(
					m.AllOf(IsIdentifyEvent(), scenario.hasExpectedUserObject(user)),
				))
			})

			t.Run("custom event", func(t *ldtest.T) {
				user := scenario.MakeUser(users.NextUniqueUser())
				eventData := ldvalue.Bool(true)
				metricValue := float64(10)
				client.SendCustomEvent(t, servicedef.CustomEventParams{
					User:        user,
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
				user := scenario.MakeUser(users.NextUniqueUser())
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

func eventUserMatcher(user lduser.User, eventsConfig servicedef.SDKConfigEventParams) m.Matcher {
	// This simulates the expected behavior of SDK event processors with regard to redacting
	// private attributes. For more details about how this works, please see the SDK
	// documentation, and/or the implementations of the equivalent logic in the SDKs
	// (such as https://github.com/launchdarkly/go-sdk-events).

	// First, get the regular JSON representation of the user, since it's simplest to treat
	// this as a transformation of one JSON object to another.
	allJSON := ldvalue.Parse(jsonhelpers.ToJSON(user))
	allAttributes := append(allJSON.Keys(), allJSON.GetByKey("custom").Keys()...)

	expected := []m.KeyValueMatcher{
		m.KV("kind", m.Equal("user")),
	}
	var private []string

	// allAttributes is now a list of all of the user's top-level properties plus all of
	// its custom attribute names. It's simplest to loop through all of those at once since
	// the logic for determining whether an attribute should be private is always the same.
	for _, attr := range allAttributes {
		if attr == "custom" || attr == "privateAttributeNames" || attr == "secondary" {
			// these aren't top-level attributes
			continue
		}
		// An attribute is private if 1. it was marked private for that particular user (as
		// reported by user.IsPrivateAttribute), 2. the SDK configuration (represented here
		// as eventsConfig) says that that particular one should always be private, or 3.
		// the SDK configuration says *all* of them should be private. Note that "key" can
		// never be private.
		isPrivate := attr != "key" && (eventsConfig.AllAttributesPrivate ||
			user.IsPrivateAttribute(lduser.UserAttribute(attr)))
		for _, pa := range eventsConfig.GlobalPrivateAttributes {
			isPrivate = isPrivate || pa == attr
		}
		if isPrivate {
			private = append(private, attr)
		} else {
			value := user.GetAttribute(lduser.UserAttribute(attr))
			expected = append(expected, m.KV(attr, m.JSONEqual(value)))
		}
	}
	secondary := user.GetSecondaryKey()
	if len(private) != 0 || secondary.IsDefined() {
		metaProps := make([]m.KeyValueMatcher, 0)
		if len(private) != 0 {
			sort.Strings(private)
			metaProps = append(metaProps, m.KV("redactedAttributes", SortedStrings().Should(m.Equal(private))))
		}
		if secondary.IsDefined() {
			metaProps = append(metaProps, m.KV("secondary", m.Equal(secondary.StringValue())))
		}
		expected = append(expected, m.KV("_meta", m.MapOf(metaProps...)))
	}
	return m.JSONMap().Should(m.MapOf(expected...))
}
