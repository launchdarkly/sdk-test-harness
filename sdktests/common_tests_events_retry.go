package sdktests

import (
	"fmt"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

// Time to wait to make sure that the SDK does not post a payload again. It is longer than the delay of
// about one second before the SDK retries a failed post.
const eventRetryCheckWindow = 2 * time.Second

// EventRetry verifies that the SDK retries a failed event post one time. HTTP 400, 408, 429, any 5xx
// status, and a connection that closes without a response are recoverable failures. The retry sends
// the same body with the same payload ID. If the retry also fails, the SDK discards the payload.
func (c CommonEventTests) EventRetry(t *ldtest.T) {
	for _, status := range []int{400, 408, 429, 500, 503} {
		t.Run(fmt.Sprintf("HTTP %d is retried once with the same payload", status), func(t *ldtest.T) {
			c.eventRetrySucceeds(t, func(events *SDKEventSink) { events.SetResponseStatusForNextPosts(status, 1) })
		})
	}

	t.Run("closed connection is retried once with the same payload", func(t *ldtest.T) {
		c.eventRetrySucceeds(t, func(events *SDKEventSink) { events.SetConnectionClosedForNextPosts(1) })
	})

	t.Run("payload is discarded if the retry also fails", c.eventRetryFails)
}

func (c CommonEventTests) eventRetrySucceeds(t *ldtest.T, failNextPost func(*SDKEventSink)) {
	client, events := c.newEventRetryClient(t)

	// Fail the first post of the payload.
	failNextPost(events)
	c.sendCustomEventWithKey(t, client, "event1")
	client.FlushEvents(t)
	failed := events.Endpoint().RequireConnection(t, defaultEventTimeout)
	retried := events.Endpoint().RequireConnection(t, defaultEventTimeout)
	payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	// The retry repeats the failed post, and the service accepts it.
	failedID := eventPayloadID(failed)
	m.In(t).For("payload ID").Assert(failedID, m.Not(m.Equal("")))
	m.In(t).For("payload ID of the retry").Assert(eventPayloadID(retried), m.Equal(failedID))
	m.In(t).For("body of the retry").Assert(string(retried.Body), m.JSONStrEqual(string(failed.Body)))
	m.In(t).Assert(payload, m.ItemsInAnyOrder(c.firstCustomEventPayloadExpectations("event1")...))

	// The SDK does not post the payload a third time, and sends later events in a new payload.
	events.Endpoint().RequireNoMoreConnections(t, eventRetryCheckWindow)
	c.expectNextEventPayload(t, client, events, "event2", failedID)
}

func (c CommonEventTests) eventRetryFails(t *ldtest.T) {
	client, events := c.newEventRetryClient(t)

	// Fail the first post of the payload and the retry.
	events.SetResponseStatusForNextPosts(503, 2)
	c.sendCustomEventWithKey(t, client, "event1")
	client.FlushEvents(t)
	failed := events.Endpoint().RequireConnection(t, defaultEventTimeout)
	retried := events.Endpoint().RequireConnection(t, defaultEventTimeout)

	// The SDK retries one time only, and then sends later events without the discarded ones.
	failedID := eventPayloadID(failed)
	m.In(t).For("payload ID of the retry").Assert(eventPayloadID(retried), m.Equal(failedID))
	events.Endpoint().RequireNoMoreConnections(t, eventRetryCheckWindow)
	c.expectNextEventPayload(t, client, events, "event2", failedID)
}

func (c CommonEventTests) newEventRetryClient(t *ldtest.T) (*SDKClient, *SDKEventSink) {
	dataSource := NewSDKDataSource(t, nil)
	events := NewSDKEventSink(t)
	// WithEventsConfig must come before the event sink, which adds the mock endpoint's base URI to the
	// events configuration.
	client := NewSDKClient(t, c.baseSDKConfigurationPlus(
		WithEventsConfig(baseEventsConfig()),
		dataSource,
		events,
	)...)
	c.discardIdentifyEventIfClientSide(t, client, events)
	return client, events
}

// expectNextEventPayload sends a custom event and verifies that the SDK posts it alone, in a payload
// with a new payload ID.
func (c CommonEventTests) expectNextEventPayload(
	t *ldtest.T,
	client *SDKClient,
	events *SDKEventSink,
	eventKey string,
	previousPayloadID string,
) {
	c.sendCustomEventWithKey(t, client, eventKey)
	client.FlushEvents(t)
	next := events.Endpoint().RequireConnection(t, defaultEventTimeout)
	payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	m.In(t).For("payload ID of the next post").Assert(eventPayloadID(next),
		m.AllOf(m.Not(m.Equal("")), m.Not(m.Equal(previousPayloadID))))
	m.In(t).For("next payload").Assert(payload, m.Items(IsCustomEventForEventKey(eventKey)))
}

// firstCustomEventPayloadExpectations returns the events of a payload with the first custom event for a
// context. A server-side SDK also sends an index event for the new context.
func (c CommonEventTests) firstCustomEventPayloadExpectations(eventKey string) []m.Matcher {
	if c.isClientSide {
		return []m.Matcher{IsCustomEventForEventKey(eventKey)}
	}
	return []m.Matcher{IsIndexEvent(), IsCustomEventForEventKey(eventKey)}
}

func (c CommonEventTests) sendCustomEventWithKey(t *ldtest.T, client *SDKClient, eventKey string) {
	params := servicedef.CustomEventParams{EventKey: eventKey}
	if !c.isClientSide {
		params.Context = o.Some(ldcontext.New("user-key"))
	}
	client.SendCustomEvent(t, params)
}

func eventPayloadID(request harness.IncomingRequestInfo) string {
	return request.Headers.Get("X-LaunchDarkly-Payload-Id")
}
