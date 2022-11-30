package sdktests

import (
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/assert"
)

func (c CommonEventTests) BufferBehavior(t *ldtest.T) {
	capacity := 20
	extraItemsOverCapacity := 3 // arbitrary non-zero value for how many events to try to add past the limit
	eventsConfig := baseEventsConfig()
	eventsConfig.Capacity = o.Some(capacity)

	context := c.contextFactory.NextUniqueContext()
	keys := make([]string, 0)
	for i := 0; i < capacity+extraItemsOverCapacity; i++ {
		keys = append(keys, fmt.Sprintf("event%d", i))
	}

	makeCustomEventExpectations := func(count int) []m.Matcher {
		var ret []m.Matcher
		if c.isClientSide {
			ret = []m.Matcher{IsIdentifyEvent()}
		} else {
			ret = []m.Matcher{IsIndexEvent()}
		}
		count -= len(ret)
		for i := 0; i < count; i++ {
			ret = append(ret, IsCustomEventForEventKey(keys[i]))
		}
		return ret
	}

	dataSource := NewSDKDataSource(t, nil)

	t.Run("capacity is enforced", func(t *ldtest.T) {
		events := NewSDKEventSink(t)
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(
			WithClientSideInitialContext(context),
			WithEventsConfig(eventsConfig),
			dataSource,
			events)...)

		for _, key := range keys {
			client.SendCustomEvent(t, servicedef.CustomEventParams{
				EventKey: key,
				Context:  o.Some(context),
			})
		}
		client.FlushEvents(t)
		payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		m.In(t).Assert(payload, m.ItemsInAnyOrder(makeCustomEventExpectations(capacity)...))
	})

	t.Run("buffer is reset after flush", func(t *ldtest.T) {
		events := NewSDKEventSink(t)
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(
			WithClientSideInitialContext(context),

			WithEventsConfig(eventsConfig),
			dataSource,
			events)...)

		for _, key := range keys {
			client.SendCustomEvent(t, servicedef.CustomEventParams{
				EventKey: key,
				Context:  o.Some(context),
			})
		}
		client.FlushEvents(t)
		payload1 := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		assert.Len(t, payload1, capacity)

		anotherKey := "one-more-event-key"
		client.SendCustomEvent(t, servicedef.CustomEventParams{
			EventKey: anotherKey,
			Context:  o.Some(context),
		})
		client.FlushEvents(t)
		payload2 := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		m.In(t).Assert(payload2, m.Items(IsCustomEventForEventKey(anotherKey)))
	})

	t.Run("summary event is still included even if buffer was full", func(t *ldtest.T) {
		// Don't need to create an actual flag, because a "flag not found" evaluation still causes a summary event
		events := NewSDKEventSink(t)
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(
			WithEventsConfig(eventsConfig),
			dataSource,
			events)...)

		// Client-side SDK will always send an initial identify event; server-side SDK will send an initial
		// index event for the user we're referencing. So that takes up 1 spot in the buffer in either case.
		keysToSend := keys[0 : len(keys)-1]
		for _, key := range keysToSend {
			client.SendCustomEvent(t, servicedef.CustomEventParams{
				EventKey: key,
				Context:  o.Some(context),
			})
		}

		_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
			FlagKey:      "arbitrary-flag-key",
			Context:      o.Some(context),
			ValueType:    servicedef.ValueTypeBool,
			DefaultValue: ldvalue.Bool(false),
		})

		client.FlushEvents(t)
		payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		expectations := append(makeCustomEventExpectations(capacity),
			IsSummaryEvent())
		m.In(t).Assert(payload, m.ItemsInAnyOrder(expectations...))
	})
}
