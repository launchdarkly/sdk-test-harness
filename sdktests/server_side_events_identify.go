package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

func doServerSideIdentifyEventTests(t *ldtest.T) {
	// These do not include detailed tests of the encoding of user attributes in identify events,
	// which are in server_side_events_users.go.
	eventsConfig := baseEventsConfig()
	users := NewUserFactory("doServerSideIdentifyEventTests",
		func(b lduser.UserBuilder) { b.Name("my favorite user") })

	dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{Events: &eventsConfig}), dataSource, events)

	t.Run("normal user", func(t *ldtest.T) {
		user := users.NextUniqueUser()
		client.SendIdentifyEvent(t, user)
		client.FlushEvents(t)
		payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
		m.In(t).Assert(payload, m.Items(
			EventIsIdentifyEvent(mockld.SimpleEventUser(user)),
		))
	})

	t.Run("user with empty key generates no event", func(t *ldtest.T) {
		keylessUser := lduser.NewUserBuilder("").Name("has a name but not a key").Build()
		client.SendIdentifyEvent(t, keylessUser)
		client.FlushEvents(t)
		events.ExpectNoAnalyticsEvents(t, time.Millisecond*200)
	})

	t.Run("identify event makes index event for same user unnecessary", func(t *ldtest.T) {
		user := users.NextUniqueUser()
		client.SendIdentifyEvent(t, user)
		client.SendCustomEvent(t, servicedef.CustomEventParams{
			EventKey: "event-key",
			User:     &user,
		})
		// Sending a custom event would also generate an index event for the user,
		// if we hadn't already seen that user
		client.FlushEvents(t)
		payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
		m.In(t).Assert(payload, m.ItemsInAnyOrder(
			EventIsIdentifyEvent(mockld.SimpleEventUser(user)),
			EventIsCustomEvent("event-key", mockld.SimpleEventUser(user), false, ldvalue.Null(), nil),
		))
	})
}
