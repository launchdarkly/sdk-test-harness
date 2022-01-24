package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"

	"github.com/stretchr/testify/assert"
)

func doServerSideEventBufferTests(t *ldtest.T) {
	capacity := 20
	extraItemsOverCapacity := 3 // arbitrary non-zero value for how many events to try to add past the limit
	eventsConfig := baseEventsConfig()
	eventsConfig.Capacity = ldvalue.NewOptionalInt(capacity)

	userFactory := NewUserFactory("doServerSideEventCapacityTests",
		func(b lduser.UserBuilder) { b.Name("my favorite user") })
	users := make([]lduser.User, 0)
	for i := 0; i < capacity+extraItemsOverCapacity; i++ {
		users = append(users, userFactory.NextUniqueUser())
	}

	makeIdentifyEventExpectations := func(count int) []m.Matcher {
		ret := make([]m.Matcher, 0, count)
		for i := 0; i < count; i++ {
			ret = append(ret, EventIsIdentifyEvent(mockld.SimpleEventUser(users[i])))
		}
		return ret
	}

	flag := ldbuilders.NewFlagBuilder("flag-key").Version(1).
		On(false).OffVariation(0).Variations(ldvalue.Bool(true)).Build()
	dataBuilder := mockld.NewServerSDKDataBuilder().Flag(flag)

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{Events: &eventsConfig}),
		dataSource, events)

	t.Run("capacity is enforced", func(t *ldtest.T) {
		for _, user := range users {
			client.SendIdentifyEvent(t, user)
		}
		client.FlushEvents(t)
		payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		m.In(t).Assert(payload, m.Items(makeIdentifyEventExpectations(capacity)...))
	})

	t.Run("buffer is reset after flush", func(t *ldtest.T) {
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

		m.In(t).Assert(payload2, m.Items(EventIsIdentifyEvent(mockld.SimpleEventUser(anotherUser))))
	})

	t.Run("summary event is still included even if buffer was full", func(t *ldtest.T) {
		for _, user := range users {
			client.SendIdentifyEvent(t, user)
		}

		_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
			FlagKey:      flag.Key,
			User:         &users[0],
			ValueType:    servicedef.ValueTypeBool,
			DefaultValue: ldvalue.Bool(false),
		})

		client.FlushEvents(t)
		payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		expectations := append(makeIdentifyEventExpectations(capacity),
			EventIsSummaryEvent())
		m.In(t).Assert(payload, m.Items(expectations...))
	})
}
