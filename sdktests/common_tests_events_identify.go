package sdktests

import (
	"time"

	h "github.com/launchdarkly/sdk-test-harness/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
)

func (c CommonEventTests) IdentifyEvents(t *ldtest.T) {
	// These do not include detailed tests of the encoding of user attributes in identify events,
	// which are in server_side_events_users.go.

	dataSource := NewSDKDataSource(t, nil)

	t.Run("basic properties", func(t *ldtest.T) {
		for _, isAnonymousUser := range []bool{false, true} {
			t.Run(h.IfElse(isAnonymousUser, "anonymous user", "non-anonymous user"), func(t *ldtest.T) {
				events := NewSDKEventSink(t)
				client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSource, events)...)

				user := c.userFactory.NextUniqueUserMaybeAnonymous(isAnonymousUser)
				client.SendIdentifyEvent(t, user)
				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.Items(
					append(c.initialEventPayloadExpectations(),
						m.AllOf(
							JSONPropertyKeysCanOnlyBe("kind", "creationDate", "key", "user"),
							IsIdentifyEventForUserKey(user.GetKey()),
							HasAnyCreationDate(),
						),
					)...,
				))
			})
		}
	})

	if !c.isClientSide {
		t.Run("user with empty key generates no event", func(t *ldtest.T) {
			// This test is only done for server-side SDKs because in client-side ones, an empty is an error.
			events := NewSDKEventSink(t)
			client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSource, events)...)

			keylessUser := lduser.NewUserBuilder("").Name("has a name but not a key").Build()
			client.SendIdentifyEvent(t, keylessUser)
			client.FlushEvents(t)
			events.ExpectNoAnalyticsEvents(t, time.Millisecond*200)
		})

		if !c.isPHP {
			t.Run("identify event makes index event for same user unnecessary", func(t *ldtest.T) {
				// This test is only done for server-side SDKs (excluding PHP), because client-side ones and PHP
				// do not do index events.
				events := NewSDKEventSink(t)
				client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSource, events)...)

				user := c.userFactory.NextUniqueUser()
				client.SendIdentifyEvent(t, user)
				client.SendCustomEvent(t, servicedef.CustomEventParams{
					EventKey: "event-key",
					User:     o.Some(user),
				})
				// Sending a custom event would also generate an index event for the user,
				// if we hadn't already seen that user
				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.ItemsInAnyOrder(
					IsIdentifyEventForUserKey(user.GetKey()),
					IsCustomEvent(),
				))
			})
		}
	}
}
