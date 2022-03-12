package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v3/lduser"
)

func doServerSideIdentifyEventTests(t *ldtest.T) {
	// These do not include detailed tests of the encoding of user attributes in identify events,
	// which are in server_side_events_users.go.
	users := NewUserFactory("doServerSideIdentifyEventTests",
		func(b lduser.UserBuilder) { b.Name("my favorite user") })

	dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSource, events)

	t.Run("basic properties", func(t *ldtest.T) {
		for _, isAnonymousUser := range []bool{false, true} {
			t.Run(selectString(isAnonymousUser, "anonymous user", "non-anonymous user"), func(t *ldtest.T) {
				user := users.NextUniqueUserMaybeAnonymous(isAnonymousUser)
				client.SendIdentifyEvent(t, user)
				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.Items(
					m.AllOf(
						JSONPropertyKeysCanOnlyBe("kind", "creationDate", "key", "context"),
						IsIdentifyEventForUserKey(user.Key()),
						HasAnyCreationDate(),
					),
				))
			})
		}
	})

	t.Run("identify event makes index event for same user unnecessary", func(t *ldtest.T) {
		user := users.NextUniqueUser()
		client.SendIdentifyEvent(t, user)
		client.SendCustomEvent(t, servicedef.CustomEventParams{
			EventKey: "event-key",
			Context:  user,
		})
		// Sending a custom event would also generate an index event for the user,
		// if we hadn't already seen that user
		client.FlushEvents(t)
		payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
		m.In(t).Assert(payload, m.ItemsInAnyOrder(
			IsIdentifyEventForUserKey(user.Key()),
			IsCustomEvent(),
		))
	})
}
