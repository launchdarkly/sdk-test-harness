package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/assert"
)

func (c CommonEventTests) BufferBehavior(t *ldtest.T) {
	capacity := 20
	extraItemsOverCapacity := 3 // arbitrary non-zero value for how many events to try to add past the limit
	eventsConfig := baseEventsConfig()
	eventsConfig.Capacity = o.Some(capacity)

	contexts := make([]ldcontext.Context, 0)
	for i := 0; i < capacity+extraItemsOverCapacity; i++ {
		contexts = append(contexts, c.contextFactory.NextUniqueContext())
	}

	// We use identify events for this test because they do not cause any other events (such as
	// index or summary) to be generated.
	makeIdentifyEventExpectations := func(count int) []m.Matcher {
		ret := c.initialEventPayloadExpectations()
		count -= len(ret)
		for i := 0; i < count; i++ {
			ret = append(ret, IsIdentifyEventForContext(contexts[i]))
		}
		return ret
	}

	dataSource := NewSDKDataSource(t, nil)

	t.Run("capacity is enforced", func(t *ldtest.T) {
		events := NewSDKEventSink(t)
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(
			WithEventsConfig(eventsConfig),
			dataSource,
			events)...)

		for _, context := range contexts {
			client.SendIdentifyEvent(t, context)
		}
		client.FlushEvents(t)
		payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		m.In(t).Assert(payload, m.ItemsInAnyOrder(makeIdentifyEventExpectations(capacity)...))
	})

	t.Run("buffer is reset after flush", func(t *ldtest.T) {
		events := NewSDKEventSink(t)
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(
			WithEventsConfig(eventsConfig),
			dataSource,
			events)...)

		for _, context := range contexts {
			client.SendIdentifyEvent(t, context)
		}
		client.FlushEvents(t)
		payload1 := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		assert.Len(t, payload1, capacity)

		anotherContext := c.contextFactory.NextUniqueContext()
		client.SendIdentifyEvent(t, anotherContext)
		client.FlushEvents(t)
		payload2 := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		m.In(t).Assert(payload2, m.Items(IsIdentifyEventForContext(anotherContext)))
	})

	t.Run("summary event is still included even if buffer was full", func(t *ldtest.T) {
		// Don't need to create an actual flag, because a "flag not found" evaluation still causes a summary event
		events := NewSDKEventSink(t)
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(
			WithEventsConfig(eventsConfig),
			dataSource,
			events)...)

		contextsToSend := contexts
		if c.isClientSide {
			// Client-side SDK always sends initial identify event, so we need to send one less identify event
			contextsToSend = contextsToSend[0 : len(contextsToSend)-1]
		}
		for _, context := range contextsToSend {
			client.SendIdentifyEvent(t, context)
		}

		_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
			FlagKey:      "arbitrary-flag-key",
			Context:      o.Some(contexts[0]),
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
