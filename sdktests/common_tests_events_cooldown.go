package sdktests

import (
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

// eventCooldownWindow is the cooldown window the event processor enters on the first unexpected
// delivery failure, per the EVTPROC spec (Section 1.8): the extended initial delay. The SDKs do not
// expose a way to shorten it, so any test that waits one out is long-running.
const eventCooldownWindow = 5 * time.Minute

// Representative HTTP statuses from each delivery-failure classification (EVTPROC Section 1.7).
// A recoverable status is retried and must not cool down; an unexpected status triggers cooldown.
const (
	recoverableEventStatus = 503
	unexpectedEventStatus  = 401
)

// noCooldownRetryTimeout is how long we wait, after an unexpected failure, to confirm the processor
// made no further post. It must comfortably exceed the processor's fixed ~1s normal retry delay, so
// that the absence of a connection means the processor entered cooldown rather than that a retry had
// simply not fired yet.
const noCooldownRetryTimeout = 3 * time.Second

// eventCooldownFlushInterval is the flush interval the cooldown tests configure. It must be shorter
// than eventCooldownWindow (the extended initial delay): Requirement 1.8.2 binds the processor's
// operating cadence to the configured flush interval, and RETRY Requirement 1.4.9 floors the wait
// before the next attempt at that cadence -- so a longer interval would push the first cooldown window
// past the extended initial delay and break the resumption test's timing. It must also exceed the fast
// tests' RequireNoMoreConnections windows so that no periodic auto-flush fires during them.
const eventCooldownFlushInterval = 60 * time.Second

// inWindowSilenceCheck is how long eventCooldownResumesAfterWindow asserts the processor stays silent
// while cooling down. RETRY jitter can only shorten a window, so a window is always at least
// eventCooldownWindow/2; keeping this comfortably under that bound means a legitimate post-window probe
// cannot be mistaken for an in-window cooldown violation.
const inWindowSilenceCheck = eventCooldownWindow/2 - 30*time.Second

// EventDeliveryCooldown verifies the event processor's cooldown behavior after an unexpected delivery
// failure, per EVTPROC Section 1.8: an unexpected failure must not permanently stop delivery; the
// processor enters a bounded cooldown window during which it does not post, and resumes afterward.
//
// Gated on the event-delivery-cooldown capability, which an SDK advertises once it implements this
// behavior; until then these tests skip.
//
// Note on coverage: the fast subtests prove that an unexpected failure causes the processor to stop
// posting for a while; they cannot, by themselves, distinguish a cooldown from a permanent disable,
// because that requires waiting out a real window (no time compression is available). The resumption
// guarantee is therefore only exercised by eventCooldownResumesAfterWindow, which is long-running. SDK
// conformance runs should enable long-running tests so that guarantee is actually checked.
func (c CommonEventTests) EventDeliveryCooldown(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityEventDeliveryCooldown)

	t.Run("413 is dropped without disabling delivery", c.eventCooldown413DoesNotDisable)
	t.Run("recoverable error does not enter cooldown", c.eventCooldownRecoverableContinues)
	t.Run("unexpected error enters cooldown", c.eventCooldownUnexpectedEntersCooldown)
	t.Run("delivery resumes after the cooldown window", c.eventCooldownResumesAfterWindow)
}

// newCooldownClient sets up a data source, an event sink, and a client configured so that the test
// controls flushing, using eventCooldownFlushInterval -- long enough that the test drives flushing, but
// short enough that it does not raise the cooldown window above the extended initial delay (see that
// constant). It also clears the initial identify event that client-side SDKs send on startup, so later
// assertions only observe events the test produced.
func (c CommonEventTests) newCooldownClient(t *ldtest.T) (*SDKClient, *SDKEventSink) {
	dataSource := NewSDKDataSource(t, nil)
	events := NewSDKEventSink(t)
	eventsConfig := baseEventsConfig()
	eventsConfig.FlushIntervalMS = o.Some(ldtime.UnixMillisecondTime(eventCooldownFlushInterval / time.Millisecond))
	client := NewSDKClient(t, c.baseSDKConfigurationPlus(
		dataSource,
		events,
		WithEventsConfig(eventsConfig),
	)...)
	c.discardIdentifyEventIfClientSide(t, client, events)
	return client, events
}

// eventCooldown413DoesNotDisable verifies that a 413 drops the payload but does not disable delivery
// or enter cooldown: a subsequent post still succeeds (EVTPROC Requirement 1.7.3.1).
func (c CommonEventTests) eventCooldown413DoesNotDisable(t *ldtest.T) {
	client, events := c.newCooldownClient(t)

	// Fail exactly the first post with a 413, after which the endpoint accepts again automatically. A
	// bounded failure (rather than SetResponseStatus + a later reset) keeps the point at which the
	// endpoint starts accepting deterministic, so it cannot race the failing post and let it through.
	events.SetResponseStatusForNextPosts(413, 1)
	c.sendArbitraryEvent(t, client)
	client.FlushEvents(t)
	events.Endpoint().RequireConnection(t, defaultEventTimeout) // the dropped post was attempted

	// The 413 payload is dropped, not retried: no further post follows it (Requirement 1.7.3.1). A
	// non-conformant SDK that retried the dropped payload would show a second connection here.
	events.Endpoint().RequireNoMoreConnections(t, noCooldownRetryTimeout)

	// Delivery is not disabled and did not cool down: a subsequent post is delivered promptly.
	c.sendArbitraryEvent(t, client)
	client.FlushEvents(t)
	events.ExpectAnalyticsEvents(t, defaultEventTimeout)
}

// eventCooldownRecoverableContinues verifies that a recoverable failure does not enter cooldown:
// delivery continues on a subsequent post (EVTPROC Requirement 1.7.2).
func (c CommonEventTests) eventCooldownRecoverableContinues(t *ldtest.T) {
	client, events := c.newCooldownClient(t)

	// Fail exactly the first post with a recoverable status, after which the endpoint accepts again
	// automatically (deterministically, without a reset that could race the failing post).
	events.SetResponseStatusForNextPosts(recoverableEventStatus, 1)
	c.sendArbitraryEvent(t, client)
	client.FlushEvents(t)
	events.Endpoint().RequireConnection(t, defaultEventTimeout) // the failing post

	// A recoverable failure must not cool down, so a subsequent post is delivered promptly rather than
	// suppressed for the length of a cooldown window. This asserts that delivery continues, not the
	// recoverable-retry mechanic itself (an SDK may also retry the failed post, which likewise delivers).
	c.sendArbitraryEvent(t, client)
	client.FlushEvents(t)
	events.ExpectAnalyticsEvents(t, defaultEventTimeout)
}

// eventCooldownUnexpectedEntersCooldown verifies that an unexpected failure makes the processor enter
// cooldown: it does not retry the failed post, and a subsequent flush during the window also no-ops
// rather than posting (EVTPROC Requirements 1.7.3, 1.8.1, 1.8.5). Entry is observed without waiting a
// full window: an unexpected failure produces no further post, unlike a recoverable one which would
// retry within ~1s.
func (c CommonEventTests) eventCooldownUnexpectedEntersCooldown(t *ldtest.T) {
	client, events := c.newCooldownClient(t)

	events.SetResponseStatus(unexpectedEventStatus)
	c.sendArbitraryEvent(t, client)
	client.FlushEvents(t)
	events.Endpoint().RequireConnection(t, defaultEventTimeout) // the failing post

	// No retry of the failed post: the processor entered cooldown rather than treating the failure as
	// recoverable. Waiting past the normal retry delay also ensures the failure has been processed
	// before the next flush below, so that flush genuinely lands inside the cooldown window.
	events.Endpoint().RequireNoMoreConnections(t, noCooldownRetryTimeout)

	// A flush during the window is a no-op: no post is made.
	c.sendArbitraryEvent(t, client)
	client.FlushEvents(t)
	events.Endpoint().RequireNoMoreConnections(t, noCooldownRetryTimeout)
}

// eventCooldownResumesAfterWindow verifies that the processor does not permanently stop: after the
// cooldown window elapses it probes again and resumes delivery (EVTPROC Requirements 1.8.1-1.8.3,
// 1.8.6). This waits out a real window, so it is long-running.
func (c CommonEventTests) eventCooldownResumesAfterWindow(t *ldtest.T) {
	t.LongRunning()
	client, events := c.newCooldownClient(t)

	// Enter cooldown with an unexpected failure.
	events.SetResponseStatus(unexpectedEventStatus)
	c.sendArbitraryEvent(t, client)
	client.FlushEvents(t)
	events.Endpoint().RequireConnection(t, defaultEventTimeout) // the failing post

	// Record an event during the window WITHOUT flushing it. The record path is not gated by cooldown
	// (only sends are), so this event is buffered rather than dropped, and must be deliverable once the
	// processor resumes. This guarantees the post-window probe has something to deliver for every SDK.
	// We deliberately do not flush it in-window: an in-window send is a no-op with a failure return
	// (Requirement 1.8.5), which a non-durable SDK would drop -- Test 3 covers that path.
	c.sendArbitraryEvent(t, client)

	// Assert real in-window silence: for an interval guaranteed to fall inside the cooldown window
	// (inWindowSilenceCheck < eventCooldownWindow/2, the jitter-guaranteed minimum), the processor makes
	// no delivery attempt. The injected failure is still in effect, so a non-conformant SDK that kept
	// retrying through the window would both fail and be caught here. This also subsumes the "cooldown
	// engaged" guard. (A violation in the [window/2, window] tail cannot be told apart from a legitimate
	// probe without time compression; Test 3 covers the immediate-retry case.)
	events.Endpoint().RequireNoMoreConnections(t, inWindowSilenceCheck)

	// Let the service accept again, then wait out the remainder of the longest possible window plus a
	// margin before probing. Clearing the failure now (rather than holding it until the probe) keeps a
	// conformant SDK's own window-end probe from hitting the injected failure and re-entering cooldown.
	events.SetResponseStatus(0)
	time.Sleep(eventCooldownWindow - inWindowSilenceCheck + 15*time.Second)

	// The probe -- the first post after the window -- is accepted, proving the processor resumed delivery
	// rather than permanently stopping. We assert only that a post is delivered, not its exact contents:
	// a durable SDK may also redeliver the retained failed payload here (possibly as a separate post)
	// while a non-durable SDK delivers only the buffered event, and both conform.
	client.FlushEvents(t)
	events.ExpectAnalyticsEvents(t, defaultEventTimeout)
}
