package sdktests

import (
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
)

type aliasEventTestScenario struct {
	params        servicedef.AliasEventParams
	expectedEvent mockld.Event
}

func doServerSideAliasEventTests(t *ldtest.T) {
	eventsConfig := baseEventsConfig()

	userFactory := NewUserFactory("doServerSideAliasEventTests")

	dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{Events: &eventsConfig}), dataSource, events)

	var scenarios []aliasEventTestScenario
	for _, user1IsAnon := range []bool{false, true} {
		for _, user2IsAnon := range []bool{false, true} {
			var scenario aliasEventTestScenario
			var newContextKind, previousContextKind = "user", "user"
			user1 := lduser.NewUserBuilderFromUser(userFactory.NextUniqueUser())
			user2 := lduser.NewUserBuilderFromUser(userFactory.NextUniqueUser())
			if user1IsAnon {
				user1.Anonymous(true)
				previousContextKind = "anonymousUser"
			} else {
				user1.Name("Mina")
			}
			if user2IsAnon {
				user2.Anonymous(true)
				newContextKind = "anonymousUser"
			} else {
				user2.Name("Lucy")
			}
			scenario.params.PreviousUser = user1.Build()
			scenario.params.User = user2.Build()
			scenario.expectedEvent = mockld.EventFromMap(map[string]interface{}{
				"kind":                "alias",
				"key":                 scenario.params.User.GetKey(),
				"previousKey":         scenario.params.PreviousUser.GetKey(),
				"contextKind":         newContextKind,
				"previousContextKind": previousContextKind,
			})
			scenarios = append(scenarios, scenario)
		}
	}
	for _, scenario := range scenarios {
		anonDesc := map[bool]string{false: "non-anonymous", true: "anonymous"}
		testDesc := fmt.Sprintf("from %s to %s", anonDesc[scenario.params.PreviousUser.GetAnonymous()],
			anonDesc[scenario.params.User.GetAnonymous()])
		t.Run(testDesc, func(t *ldtest.T) {
			client.SendAliasEvent(t, scenario.params)
			client.FlushEvents(t)
			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
			m.AssertThat(t, payload, m.Items(
				CanonicalizedEventJSON().Should(m.JSONEqual(scenario.expectedEvent.AsValue())),
			))
		})
	}
}