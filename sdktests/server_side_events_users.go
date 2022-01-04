package sdktests

import (
	"fmt"
	"strings"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
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
				for _, userPrivateAttrs := range [][]lduser.UserAttribute{nil, {lduser.LastNameAttribute}} {
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
		func(ub lduser.UserBuilder) { ub.FirstName("first").LastName("last").Country("us") })
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
			client := NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{
				Events: &scenario.config,
			}), dataSource, events)

			t.Run("feature event", func(t *ldtest.T) {
				user := scenario.MakeUser(users.NextUniqueUser())
				client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
					FlagKey:      flag.Key,
					User:         &user,
					ValueType:    servicedef.ValueTypeAny,
					DefaultValue: defaultValue,
				})
				client.FlushEvents(t)

				eventUser := mockld.ExpectedEventUserFromUser(user, scenario.config)
				matchFeatureEvent := EventIsFeatureEvent(
					flag.Key,
					eventUser,
					scenario.config.InlineUsers,
					ldvalue.NewOptionalInt(flag.Version),
					flagValue,
					ldvalue.NewOptionalInt(0),
					ldreason.EvaluationReason{},
					defaultValue,
				)

				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				if scenario.config.InlineUsers {
					m.AssertThat(t, payload, m.ItemsInAnyOrder(
						matchFeatureEvent,
						EventHasKind("summary"),
					))
				} else {
					m.AssertThat(t, payload, m.ItemsInAnyOrder(
						EventIsIndexEvent(eventUser),
						matchFeatureEvent,
						EventHasKind("summary"),
					))
				}
			})

			t.Run("identify event", func(t *ldtest.T) {
				user := scenario.MakeUser(users.NextUniqueUser())
				client.SendIdentifyEvent(t, user)
				client.FlushEvents(t)

				eventUser := mockld.ExpectedEventUserFromUser(user, scenario.config)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.AssertThat(t, payload, m.Items(
					EventIsIdentifyEvent(eventUser)))
			})

			t.Run("custom event", func(t *ldtest.T) {
				user := scenario.MakeUser(users.NextUniqueUser())
				eventData := ldvalue.Bool(true)
				metricValue := float64(10)
				client.SendCustomEvent(t, servicedef.CustomEventParams{
					User:        &user,
					EventKey:    "event-key",
					Data:        eventData,
					MetricValue: &metricValue,
				})
				client.FlushEvents(t)

				eventUser := mockld.ExpectedEventUserFromUser(user, scenario.config)
				matchCustomEvent := EventIsCustomEvent(
					"event-key",
					eventUser,
					scenario.config.InlineUsers,
					eventData,
					&metricValue,
				)

				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				if scenario.config.InlineUsers {
					m.AssertThat(t, payload, m.Items(matchCustomEvent))
				} else {
					m.AssertThat(t, payload, m.ItemsInAnyOrder(
						EventIsIndexEvent(eventUser),
						matchCustomEvent,
					))
				}
			})
		})
	}
}
