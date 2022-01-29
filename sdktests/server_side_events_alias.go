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
	params       servicedef.AliasEventParams
	eventMatcher m.Matcher
}

func doServerSideAliasEventTests(t *ldtest.T) {
	userFactory := NewUserFactory("doServerSideAliasEventTests")

	dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSource, events)

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
		anonDesc := map[bool]string{false: "non-anonymous", true: "anonymous"}
		testDesc := fmt.Sprintf("from %s to %s", anonDesc[scenario.params.PreviousUser.GetAnonymous()],
			anonDesc[scenario.params.User.GetAnonymous()])
		t.Run(testDesc, func(t *ldtest.T) {
			client.SendAliasEvent(t, scenario.params)
			client.FlushEvents(t)
			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
			m.In(t).Assert(payload, m.Items(scenario.eventMatcher))
		})
	}
}
