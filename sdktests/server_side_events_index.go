package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

func doServerSideIndexEventTests(t *ldtest.T) {
	// These do not include detailed tests of the properties within the user object, which are in
	// server_side_events_users.go.

	users := NewUserFactory("doServerSideIndexEventTests")
	matchIndexEvent := func(user lduser.User) m.Matcher {
		return m.AllOf(
			JSONPropertyKeysCanOnlyBe("kind", "creationDate", "user"),
			IsIndexEvent(),
			HasAnyCreationDate(),
			HasUserObjectWithKey(user.GetKey()),
		)
	}

	t.Run("basic properties", func(t *ldtest.T) {
		dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
		events := NewSDKEventSink(t)
		client := NewSDKClient(t, dataSource, events)

		for _, isAnonymousUser := range []bool{false, true} {
			t.Run(selectString(isAnonymousUser, "anonymous user", "non-anonymous user"), func(t *ldtest.T) {
				user := users.NextUniqueUserMaybeAnonymous(isAnonymousUser)

				basicEvaluateFlag(t, client, "arbitrary-flag-key", user, ldvalue.Null())

				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

				m.In(t).Assert(payload, m.Items(
					matchIndexEvent(user),
					IsSummaryEvent(),
				))
			})
		}
	})

	t.Run("only one index event per user", func(t *ldtest.T) {
		dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())

		t.Run("from feature event", func(t *ldtest.T) {
			events := NewSDKEventSink(t)
			client := NewSDKClient(t, dataSource, events)

			user1 := users.NextUniqueUser()
			user2 := users.NextUniqueUser()
			flagKey := "arbitrary-flag-key"

			basicEvaluateFlag(t, client, flagKey, user1, ldvalue.Null())
			basicEvaluateFlag(t, client, flagKey, user1, ldvalue.Null())
			basicEvaluateFlag(t, client, flagKey, user2, ldvalue.Null())
			basicEvaluateFlag(t, client, flagKey, user1, ldvalue.Null())
			basicEvaluateFlag(t, client, flagKey, user2, ldvalue.Null())

			client.FlushEvents(t)
			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

			m.In(t).Assert(payload, m.ItemsInAnyOrder(
				matchIndexEvent(user1),
				matchIndexEvent(user2),
				IsSummaryEvent(),
			))
		})

		t.Run("from custom event", func(t *ldtest.T) {
			events := NewSDKEventSink(t)
			client := NewSDKClient(t, dataSource, events)

			user1 := users.NextUniqueUser()
			user2 := users.NextUniqueUser()
			params1 := servicedef.CustomEventParams{EventKey: "event1", User: user1}
			params2 := servicedef.CustomEventParams{EventKey: "event1", User: user2}

			client.SendCustomEvent(t, params1)
			client.SendCustomEvent(t, params1)
			client.SendCustomEvent(t, params2)
			client.SendCustomEvent(t, params1)
			client.SendCustomEvent(t, params2)

			client.FlushEvents(t)
			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

			m.In(t).Assert(payload, m.ItemsInAnyOrder(
				matchIndexEvent(user1),
				matchIndexEvent(user2),
				IsCustomEvent(), IsCustomEvent(), IsCustomEvent(), IsCustomEvent(), IsCustomEvent(),
			))
		})
	})
}