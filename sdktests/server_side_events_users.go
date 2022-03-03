package sdktests

import (
	"fmt"
	"sort"
	"strings"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
)

type eventUserTestScenario struct {
	config           servicedef.SDKConfigEventParams
	userPrivateAttrs []lduser.UserAttribute
}

func (s eventUserTestScenario) MakeUser(originalUser lduser.User) lduser.User {
	if len(s.userPrivateAttrs) == 0 {
		return originalUser
	}
	ub := lduser.NewUserBuilderFromUser(originalUser)
	for _, a := range s.userPrivateAttrs {
		ub.SetAttribute(a, originalUser.GetAttribute(a)).AsPrivateAttribute()
	}
	return ub.Build()
}

func (s eventUserTestScenario) hasExpectedUserObject(user lduser.User) m.Matcher {
	return m.JSONProperty("user").Should(eventUserMatcher(user, s.config))
}

func (s eventUserTestScenario) Description() string {
	parts := []string{
		fmt.Sprintf("inlineUsers=%t", s.config.InlineUsers),
	}
	if s.config.AllAttributesPrivate {
		parts = append(parts, "allAttributesPrivate=true")
	}
	if len(s.config.GlobalPrivateAttributes) != 0 {
		parts = append(parts, fmt.Sprintf("globally-private=%v", s.config.GlobalPrivateAttributes))
	}
	if len(s.userPrivateAttrs) != 0 {
		parts = append(parts, fmt.Sprintf("user-private=%v", s.userPrivateAttrs))
	}
	return strings.Join(parts, ", ")
}

func doServerSideEventUserTests(t *ldtest.T) {
	var scenarios []eventUserTestScenario
	for _, inlineUsers := range []bool{false, true} {
		for _, allAttrsPrivate := range []bool{false, true} {
			for _, globalPrivateAttrs := range [][]lduser.UserAttribute{nil, {lduser.FirstNameAttribute}} {
				for _, userPrivateAttrs := range [][]lduser.UserAttribute{nil, {lduser.LastNameAttribute, "preferredLanguage"}} {
					scenarios = append(scenarios, eventUserTestScenario{
						config: servicedef.SDKConfigEventParams{
							InlineUsers:             inlineUsers,
							AllAttributesPrivate:    allAttrsPrivate,
							GlobalPrivateAttributes: globalPrivateAttrs,
						},
						userPrivateAttrs: userPrivateAttrs,
					})
				}
			}
		}
	}

	flagValue := ldvalue.String("value")
	defaultValue := ldvalue.String("default")
	users := NewUserFactory("doServerSideEventUserTests",
		func(ub lduser.UserBuilder) {
			ub.FirstName("first").LastName("last").Country("us").Custom("preferredLanguage", ldvalue.String("go")).Custom("primaryLanguage", ldvalue.String("go"))
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
				if scenario.config.InlineUsers {
					m.In(t).Assert(payload, m.ItemsInAnyOrder(
						m.AllOf(IsFeatureEvent(), scenario.hasExpectedUserObject(user), HasNoUserKeyProperty()),
						IsSummaryEvent(),
					))
				} else {
					m.In(t).Assert(payload, m.ItemsInAnyOrder(
						IsIndexEvent(),
						m.AllOf(IsFeatureEvent(), HasNoUserObject(), HasUserKeyProperty(user.GetKey())),
						IsSummaryEvent(),
					))
				}
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
				if scenario.config.InlineUsers {
					m.In(t).Assert(payload, m.Items(
						m.AllOf(IsCustomEvent(), scenario.hasExpectedUserObject(user)),
					))
				} else {
					m.In(t).Assert(payload, m.ItemsInAnyOrder(
						m.AllOf(IsIndexEvent()),
						m.AllOf(IsCustomEvent(), HasNoUserObject(), HasUserKeyProperty(user.GetKey())),
					))
				}
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
	custom := make(map[string]ldvalue.Value)
	var private []string
	allAttributes := append(allJSON.Keys(), allJSON.GetByKey("custom").Keys()...)

	var conditions []m.Matcher
	allowedTopLevelKeys := []string{"custom"}

	// allAttributes is now a list of all of the user's top-level properties plus all of
	// its custom attribute names. It's simplest to loop through all of those at once since
	// the logic for determining whether an attribute should be private is always the same.
	for _, attr := range allAttributes {
		if attr == "custom" || attr == "privateAttributeNames" {
			// "custom" and "privateAttributeNames" aren't considered user attributes, they
			// are just details of the JSON schema
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
			isPrivate = isPrivate || string(pa) == attr
		}
		if isPrivate {
			private = append(private, attr)
		} else {
			value := user.GetAttribute(lduser.UserAttribute(attr))
			if _, isTopLevel := allJSON.TryGetByKey(attr); isTopLevel {
				conditions = append(conditions, m.JSONProperty(attr).Should(m.JSONEqual(value)))
				allowedTopLevelKeys = append(allowedTopLevelKeys, attr)
			} else {
				custom[attr] = value
			}
		}
	}
	if len(custom) == 0 {
		// if there are no custom properties, the SDK is allowed to write "custom":{}, "custom":null,
		// or just omit the property
		conditions = append(conditions,
			m.JSONOptProperty("custom").Should(m.AnyOf(
				m.BeNil(),
				m.JSONStrEqual("{}"),
			)),
		)
	} else {
		conditions = append(conditions, m.JSONProperty("custom").Should(m.JSONEqual(custom)))
	}
	if len(private) != 0 {
		// the SDK should only send "privateAttrs" if there are some private attributes
		allowedTopLevelKeys = append(allowedTopLevelKeys, "privateAttrs")
		sort.Strings(private)
		conditions = append(conditions, m.JSONProperty("privateAttrs").Should(SortedStrings().Should(m.Equal(private))))
	}

	conditions = append(conditions, JSONPropertyKeysCanOnlyBe(allowedTopLevelKeys...))
	return m.AllOf(conditions...)
}
