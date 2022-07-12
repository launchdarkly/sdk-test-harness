package sdktests

import (
	"fmt"
	"strings"

	h "github.com/launchdarkly/sdk-test-harness/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
)

func (c CommonEventTests) EventUsers(t *ldtest.T) {
	users := NewUserFactory(c.userFactory.prefix,
		func(ub lduser.UserBuilder) {
			ub.FirstName("first").LastName("last").Country("us").
				Custom("preferredLanguage", ldvalue.String("go")).Custom("primaryLanguage", ldvalue.String("go"))
		})

	eventConfigPermutations := c.makeEventConfigPermutations()

	for _, eventsConfig := range eventConfigPermutations {
		t.Run(c.describeEventConfig(eventsConfig), func(t *ldtest.T) {
			c.eventUsersWithConfig(t, eventsConfig, users)
		})
	}
}

func (c CommonEventTests) eventUsersWithConfig(
	t *ldtest.T,
	eventsConfig servicedef.SDKConfigEventParams,
	users *UserFactory,
) {
	flagKey := "flag-key"
	data := c.makeSDKDataWithTrackedFlag(flagKey)
	dataSource := NewSDKDataSource(t, data)

	events := NewSDKEventSink(t)
	client := NewSDKClient(t, c.baseSDKConfigurationPlus(
		WithEventsConfig(eventsConfig),
		dataSource,
		events)...)

	c.discardIdentifyEventIfClientSide(t, client, events) // client-side SDKs always send an initial identify

	userPrivateAttrPermutations := [][]lduser.UserAttribute{
		nil,
		{lduser.LastNameAttribute, "preferredLanguage"},
	}

	for _, userPrivateAttrs := range userPrivateAttrPermutations {
		makeUser := func() lduser.User {
			user := users.NextUniqueUser()
			if len(userPrivateAttrs) == 0 {
				return user
			}
			ub := lduser.NewUserBuilderFromUser(user)
			for _, a := range userPrivateAttrs {
				ub.SetAttribute(a, user.GetAttribute(a)).AsPrivateAttribute()
			}
			return ub.Build()
		}

		desc := fmt.Sprintf("user-private=%v", h.IfElse[interface{}](len(userPrivateAttrs) == 0, "none",
			userPrivateAttrs))

		maybeWithIndexEvent := func(matchers ...m.Matcher) []m.Matcher {
			// Server-side SDKs send an index event for each never-before-seen user. Client-side SDKs do not.
			if c.isClientSide {
				return matchers
			}
			return append([]m.Matcher{IsIndexEvent()}, matchers...)
		}

		t.Run(desc, func(t *ldtest.T) {
			t.Run("identify event", func(t *ldtest.T) {
				user := makeUser()
				client.SendIdentifyEvent(t, user)

				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.Items(
					m.AllOf(IsIdentifyEvent(), eventUserMatcher(user, eventsConfig, c.isMobile)),
				))
			})

			t.Run("feature event", func(t *ldtest.T) {
				user := makeUser()

				if c.isClientSide {
					// For client-side SDKs, we must call Identify first to set the current user that will be
					// used in the evaluation; we'll discard the resulting identify event.
					client.SendIdentifyEvent(t, user)
					c.discardIdentifyEventIfClientSide(t, client, events)
				}

				_ = basicEvaluateFlag(t, client, flagKey, user, ldvalue.String("default"))
				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

				if eventsConfig.InlineUsers {
					m.In(t).Assert(payload, m.ItemsInAnyOrder(
						m.AllOf(IsFeatureEvent(), eventUserMatcher(user, eventsConfig, c.isMobile), HasNoUserKeyProperty()),
						IsSummaryEvent(),
					))
				} else {
					m.In(t).Assert(payload, m.ItemsInAnyOrder(
						maybeWithIndexEvent(
							m.AllOf(IsFeatureEvent(), HasNoUserObject(), HasUserKeyProperty(user.GetKey())),
							IsSummaryEvent(),
						)...,
					))
				}
			})

			t.Run("custom event", func(t *ldtest.T) {
				user := makeUser()
				eventData := ldvalue.Bool(true)
				metricValue := float64(10)

				if c.isClientSide {
					// For client-side SDKs, we must call Identify first to set the current user that will be
					// used in the custom event; we'll discard the resulting identify event.
					client.SendIdentifyEvent(t, user)
					c.discardIdentifyEventIfClientSide(t, client, events)
				}

				client.SendCustomEvent(t, servicedef.CustomEventParams{
					User:        o.Some(user),
					EventKey:    "event-key",
					Data:        eventData,
					MetricValue: o.Some(metricValue),
				})
				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

				if eventsConfig.InlineUsers {
					m.In(t).Assert(payload, m.Items(
						m.AllOf(IsCustomEvent(), eventUserMatcher(user, eventsConfig, c.isMobile)),
					))
				} else {
					m.In(t).Assert(payload, m.ItemsInAnyOrder(
						maybeWithIndexEvent(
							m.AllOf(IsCustomEvent(), HasNoUserObject(), HasUserKeyProperty(user.GetKey())))...))
				}
			})

			if !c.isClientSide {
				t.Run("index event", func(t *ldtest.T) {
					// Doing an evaluation for a never-before-seen user will generate an index event. We don't
					// care about the evaluation result or the summary data, we're just looking at the user
					// properties in the index event itself.
					user := makeUser()
					basicEvaluateFlag(t, client, "arbitrary-flag-key", user, ldvalue.Null())
					client.FlushEvents(t)

					payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
					m.In(t).Assert(payload, m.ItemsInAnyOrder(
						m.AllOf(IsIndexEvent(), eventUserMatcher(user, eventsConfig, c.isMobile)),
						IsSummaryEvent(),
					))
				})
			}
		})
	}
}

func (c CommonEventTests) makeSDKDataWithTrackedFlag(flagKey string) mockld.SDKData {
	// This sets up the SDK data so that evaluating this flag will produce a full feature event.
	// The flag variation/value is irrelevant.
	flagValue := ldvalue.String("value")

	if c.isClientSide {
		return mockld.NewClientSDKDataBuilder().
			Flag(flagKey, mockld.ClientSDKFlag{
				Value:       flagValue,
				Variation:   o.Some(0),
				TrackEvents: true,
			}).
			Build()
	}

	flags := FlagFactoryForValueTypes{
		ValueFactory:   SingleValueFactory(flagValue),
		BuilderActions: func(b *ldbuilders.FlagBuilder) { b.TrackEvents(true) },
	}
	flag := flags.ForType(servicedef.ValueTypeAny)
	flag.Key = flagKey
	return mockld.NewServerSDKDataBuilder().Flag(flag).Build()
}

func eventUserMatcher(user lduser.User, eventsConfig servicedef.SDKConfigEventParams, isMobile bool) m.Matcher {
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

	conditions = append(conditions, JSONUserCustomAttributesProperty(custom, isMobile))

	if len(private) != 0 || (eventsConfig.AllAttributesPrivate && isMobile) {
		// The SDK should only send "privateAttrs" if there are some private attributes. If it is a mobile
		// SDK, then the SDK may have also added "device" and "os" attributes, which will become private if
		// allAttributesPrivate was set.
		allowedTopLevelKeys = append(allowedTopLevelKeys, "privateAttrs")
		var matcher m.Matcher
		if isMobile && eventsConfig.AllAttributesPrivate {
			matcher = SortedStrings().Should(
				m.AnyOf(
					m.Equal(h.Sorted(private)),
					m.Equal(h.Sorted(append(h.CopyOf(private), "device", "os"))),
				),
			)
		} else {
			matcher = SortedStrings().Should(m.Equal(h.Sorted(private)))
		}
		conditions = append(conditions, m.JSONProperty("privateAttrs").Should(matcher))
	}

	conditions = append(conditions, JSONPropertyKeysCanOnlyBe(allowedTopLevelKeys...))

	return m.JSONProperty("user").Should(m.AllOf(conditions...))
}

func (c CommonEventTests) makeEventConfigPermutations() []servicedef.SDKConfigEventParams {
	var ret []servicedef.SDKConfigEventParams
	for _, inlineUsers := range []bool{false, true} {
		for _, allAttrsPrivate := range []bool{false, true} {
			for _, globalPrivateAttrs := range [][]lduser.UserAttribute{nil, {lduser.FirstNameAttribute}} {
				ret = append(ret, servicedef.SDKConfigEventParams{
					InlineUsers:             inlineUsers,
					AllAttributesPrivate:    allAttrsPrivate,
					GlobalPrivateAttributes: globalPrivateAttrs,
				})
			}
		}
	}
	return ret
}

func (c CommonEventTests) describeEventConfig(config servicedef.SDKConfigEventParams) string {
	parts := []string{
		fmt.Sprintf("inlineUsers=%t", config.InlineUsers),
	}
	if config.AllAttributesPrivate {
		parts = append(parts, "allAttributesPrivate=true")
	}
	if len(config.GlobalPrivateAttributes) != 0 {
		parts = append(parts, fmt.Sprintf("globally-private=%v", config.GlobalPrivateAttributes))
	}
	return strings.Join(parts, ", ")
}
