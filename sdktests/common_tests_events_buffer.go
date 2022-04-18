package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"

	"github.com/stretchr/testify/assert"
)

func (c CommonEventTests) BufferBehavior(t *ldtest.T, userFactory *UserFactory) {
	capacity := 20
	extraItemsOverCapacity := 3 // arbitrary non-zero value for how many events to try to add past the limit
	eventsConfig := baseEventsConfig()
	eventsConfig.Capacity = o.Some(capacity)

	users := make([]lduser.User, 0)
	for i := 0; i < capacity+extraItemsOverCapacity; i++ {
		users = append(users, userFactory.NextUniqueUser())
	}

	// We use identify events for this test because they do not cause any other events (such as
	// index or summary) to be generated.
	makeIdentifyEventExpectations := func(count int) []m.Matcher {
		ret := make([]m.Matcher, 0, count)
		if t.Capabilities().Has(servicedef.CapabilityClientSide) {
			// Client-side SDK always sends initial identify event
			ret = append(ret, IsIdentifyEvent())
			count--
		}
		for i := 0; i < count; i++ {
			ret = append(ret, IsIdentifyEventForUserKey(users[i].GetKey()))
		}
		return ret
	}

	dataSource := NewSDKDataSource(t, nil)

	t.Run("capacity is enforced", func(t *ldtest.T) {
		events := NewSDKEventSink(t)
		client := NewSDKClient(t,
			append(c.SDKConfigurers,
				WithEventsConfig(eventsConfig),
				dataSource,
				events)...)

		for _, user := range users {
			client.SendIdentifyEvent(t, user)
		}
		client.FlushEvents(t)
		payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		m.In(t).Assert(payload, m.ItemsInAnyOrder(makeIdentifyEventExpectations(capacity)...))
	})

	t.Run("buffer is reset after flush", func(t *ldtest.T) {
		events := NewSDKEventSink(t)
		client := NewSDKClient(t,
			append(c.SDKConfigurers,
				WithEventsConfig(eventsConfig),
				dataSource,
				events)...)

		for _, user := range users {
			client.SendIdentifyEvent(t, user)
		}
		client.FlushEvents(t)
		payload1 := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		assert.Len(t, payload1, capacity)

		anotherUser := userFactory.NextUniqueUser()
		client.SendIdentifyEvent(t, anotherUser)
		client.FlushEvents(t)
		payload2 := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		m.In(t).Assert(payload2, m.Items(IsIdentifyEventForUserKey(anotherUser.GetKey())))
	})

	t.Run("summary event is still included even if buffer was full", func(t *ldtest.T) {
		// Don't need to create an actual flag, because a "flag not found" evaluation still causes a summary event
		events := NewSDKEventSink(t)
		client := NewSDKClient(t,
			append(c.SDKConfigurers,
				WithEventsConfig(eventsConfig),
				dataSource,
				events)...)

		usersToSend := users
		if t.Capabilities().Has(servicedef.CapabilityClientSide) {
			// Client-side SDK always sends initial identify event, so we need to send one less identify event
			usersToSend = usersToSend[0 : len(usersToSend)-1]
		}
		for _, user := range usersToSend {
			client.SendIdentifyEvent(t, user)
		}

		_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
			FlagKey:      "arbitrary-flag-key",
			User:         o.Some(users[0]),
			ValueType:    servicedef.ValueTypeBool,
			DefaultValue: ldvalue.Bool(false),
		})

		client.FlushEvents(t)
		payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		expectations := append(makeIdentifyEventExpectations(capacity),
			IsSummaryEvent())
		m.In(t).Assert(payload, m.ItemsInAnyOrder(expectations...))
	})
}
