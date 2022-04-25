package sdktests

import (
	"fmt"

	h "github.com/launchdarkly/sdk-test-harness/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
)

func (c CommonEventTests) AliasEvents(t *ldtest.T) {
	dataSource := NewSDKDataSource(t, nil)

	type aliasEventTestScenario struct {
		params       servicedef.AliasEventParams
		eventMatcher m.Matcher
	}

	var scenarios []aliasEventTestScenario
	for _, user1IsAnon := range []bool{false, true} {
		for _, user2IsAnon := range []bool{false, true} {
			var scenario aliasEventTestScenario
			var newContextKind, previousContextKind = "user", "user"
			user1 := lduser.NewUserBuilderFromUser(c.userFactory.NextUniqueUser())
			user2 := lduser.NewUserBuilderFromUser(c.userFactory.NextUniqueUser())
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
			scenario.eventMatcher = m.AllOf(
				JSONPropertyKeysCanOnlyBe("kind", "creationDate", "key", "previousKey", "contextKind", "previousContextKind"),
				IsAliasEvent(),
				HasAnyCreationDate(),
				m.JSONProperty("key").Should(m.Equal(scenario.params.User.GetKey())),
				m.JSONProperty("previousKey").Should(m.Equal(scenario.params.PreviousUser.GetKey())),
				m.JSONProperty("contextKind").Should(m.Equal(newContextKind)),
				m.JSONProperty("previousContextKind").Should(m.Equal(previousContextKind)),
			)
			scenarios = append(scenarios, scenario)
		}
	}
	for _, scenario := range scenarios {
		anonDesc := func(isAnon bool) string { return h.IfElse(isAnon, "anonymous", "non-anonymous") }
		testDesc := fmt.Sprintf("from %s to %s", anonDesc(scenario.params.PreviousUser.GetAnonymous()),
			anonDesc(scenario.params.User.GetAnonymous()))

		t.Run(testDesc, func(t *ldtest.T) {
			events := NewSDKEventSink(t)
			client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSource, events)...)

			client.SendAliasEvent(t, scenario.params)
			client.FlushEvents(t)
			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

			m.In(t).Assert(payload, m.Items(
				append(c.initialEventPayloadExpectations(), scenario.eventMatcher)...))
		})
	}
}
