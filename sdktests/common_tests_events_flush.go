package sdktests

import (
	"fmt"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"

	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/require"
)

// The flush interval that the automatic flush tests configure.
const automaticFlushInterval = time.Second

// Time to wait for an automatic flush. Some SDKs enforce a minimum flush interval, for example 2 seconds
// in JS-based client-side SDKs. This timeout allows for that minimum.
const automaticFlushTimeout = 4 * time.Second

// AutomaticFlush verifies that the SDK posts its events at the configured flush interval, with no
// flush command from the test.
func (c CommonEventTests) AutomaticFlush(t *ldtest.T) {
	t.Run("events are flushed automatically at the flush interval", func(t *ldtest.T) {
		client, events := c.newAutomaticFlushClient(t)

		// Two rounds show that the automatic flush repeats.
		for _, eventKey := range []string{"event1", "event2"} {
			sentAt := time.Now()
			c.sendCustomEventWithKey(t, client, eventKey)
			expectEventInPayloadWithin(t, events, IsCustomEventForEventKey(eventKey), automaticFlushTimeout,
				fmt.Sprintf("custom event %q", eventKey))
			t.Debug("event %q arrived %s after it was sent", eventKey, time.Since(sentAt))
		}
	})

	t.Run("automatic flushing continues after failed posts", func(t *ldtest.T) {
		client, events := c.newAutomaticFlushClient(t)

		// Fail the first automatic post and its retry.
		events.SetResponseStatusForNextPosts(503, 2)
		c.sendCustomEventWithKey(t, client, "event1")
		failed := events.Endpoint().RequireConnection(t, automaticFlushTimeout)
		retried := events.Endpoint().RequireConnection(t, defaultEventTimeout)
		m.In(t).For("payload ID of the retry").Assert(eventPayloadID(retried), m.Equal(eventPayloadID(failed)))

		// The SDK continues to post events at the flush interval.
		c.sendCustomEventWithKey(t, client, "event2")
		expectEventInPayloadWithin(t, events, IsCustomEventForEventKey("event2"), automaticFlushTimeout,
			`custom event "event2"`)
	})
}

func (c CommonEventTests) newAutomaticFlushClient(t *ldtest.T) (*SDKClient, *SDKEventSink) {
	dataSource := NewSDKDataSource(t, nil)
	events := NewSDKEventSink(t)
	eventsConfig := baseEventsConfig()
	eventsConfig.FlushIntervalMS = o.Some(ldtime.UnixMillisecondTime(automaticFlushInterval / time.Millisecond))
	// WithEventsConfig must come before the event sink, which adds the mock endpoint's base URI to the
	// events configuration.
	client := NewSDKClient(t, c.baseSDKConfigurationPlus(
		WithEventsConfig(eventsConfig),
		dataSource,
		events,
	)...)
	c.discardIdentifyEventIfClientSide(t, client, events)
	return client, events
}

// expectEventInPayloadWithin waits for an analytics event payload that contains a matching event. It
// skips payloads that do not contain one, and fails the test if no such payload arrives in time.
func expectEventInPayloadWithin(
	t *ldtest.T,
	events *SDKEventSink,
	matcher m.Matcher,
	timeout time.Duration,
	description string,
) {
	deadline := time.Now().Add(timeout)
	for {
		payload, ok := events.Service().AwaitAnalyticsEventPayload(time.Until(deadline))
		if !ok {
			require.Fail(t, "timed out waiting for an automatic flush",
				"expected a payload with %s within %s", description, timeout)
		}
		for _, event := range payload {
			if pass, _ := matcher.Test(event); pass {
				return
			}
		}
		t.Debug("skipped a payload that does not contain the expected event")
	}
}
