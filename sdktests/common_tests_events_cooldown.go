package sdktests

import (
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

// Cooldown window entered on the first unexpected failure (EVTPROC §1.8 extended initial delay). Not
// shortenable, so any test that waits one out is long-running.
const eventCooldownWindow = 5 * time.Minute

// Representative recoverable vs. unexpected delivery-failure statuses (EVTPROC §1.7).
const (
	recoverableEventStatus = 503
	unexpectedEventStatus  = 401
)

// Wait used to confirm no further post was made. Must exceed the fixed ~1s normal retry delay, so the
// absence of a connection means cooldown rather than a retry that simply has not fired yet.
const noCooldownRetryTimeout = 3 * time.Second

// Flush interval for the cooldown tests. Must be shorter than eventCooldownWindow: §1.8.2 ties the
// operating cadence to the flush interval and RETRY §1.4.9 floors the window at that cadence, so a
// larger interval would stretch the first window past the extended initial delay. Must also exceed the
// fast tests' RequireNoMoreConnections windows so no periodic auto-flush fires during them.
const eventCooldownFlushInterval = 60 * time.Second

// In-window silence asserted by eventCooldownResumesAfterWindow. Jitter only shortens a window, so a
// window is always at least eventCooldownWindow/2; staying under that keeps a legitimate post-window
// probe from being mistaken for an in-window violation.
const inWindowSilenceCheck = eventCooldownWindow/2 - 30*time.Second

// EventDeliveryCooldown verifies the event processor's cooldown after an unexpected delivery failure
// (EVTPROC §1.8): delivery is never permanently stopped; the processor enters a bounded window and
// resumes afterward. Gated on the event-delivery-cooldown capability. Only the long-running resumption
// subtest proves resumption (no time compression), so conformance runs should enable long-running tests.
func (c CommonEventTests) EventDeliveryCooldown(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityEventDeliveryCooldown)

	t.Run("413 is dropped without disabling delivery", c.eventCooldown413DoesNotDisable)
	t.Run("recoverable error does not enter cooldown", c.eventCooldownRecoverableContinues)
	t.Run("unexpected error enters cooldown", c.eventCooldownUnexpectedEntersCooldown)
	t.Run("delivery resumes after the cooldown window", c.eventCooldownResumesAfterWindow)
}

// newCooldownClient builds a client whose flushing the test drives (via eventCooldownFlushInterval) and
// clears the startup identify event that client-side SDKs send.
func (c CommonEventTests) newCooldownClient(t *ldtest.T) (*SDKClient, *SDKEventSink) {
	dataSource := NewSDKDataSource(t, nil)
	events := NewSDKEventSink(t)
	eventsConfig := baseEventsConfig()
	eventsConfig.FlushIntervalMS = o.Some(ldtime.UnixMillisecondTime(eventCooldownFlushInterval / time.Millisecond))
	// WithEventsConfig must precede the event sink: configurers apply in order, and the sink merges
	// the mock endpoint's base URI into the events config, which WithEventsConfig would overwrite.
	client := NewSDKClient(t, c.baseSDKConfigurationPlus(
		WithEventsConfig(eventsConfig),
		dataSource,
		events,
	)...)
	c.discardIdentifyEventIfClientSide(t, client, events)
	return client, events
}

// eventCooldown413DoesNotDisable: a 413 drops the payload without retrying it or disabling delivery
// (EVTPROC Requirement 1.7.3.1).
func (c CommonEventTests) eventCooldown413DoesNotDisable(t *ldtest.T) {
	client, events := c.newCooldownClient(t)

	// Fail only the first post; a bounded failure resumes acceptance deterministically, with no reset
	// that could race the failing post.
	events.SetResponseStatusForNextPosts(413, 1)
	c.sendArbitraryEvent(t, client)
	client.FlushEvents(t)
	events.Endpoint().RequireConnection(t, defaultEventTimeout)           // dropped post attempted
	events.Endpoint().RequireNoMoreConnections(t, noCooldownRetryTimeout) // not retried

	// Delivery is not disabled: a later post still gets through.
	c.sendArbitraryEvent(t, client)
	client.FlushEvents(t)
	events.ExpectAnalyticsEvents(t, defaultEventTimeout)
}

// eventCooldownRecoverableContinues: a recoverable failure does not cool down; delivery continues
// (EVTPROC Requirement 1.7.2).
func (c CommonEventTests) eventCooldownRecoverableContinues(t *ldtest.T) {
	client, events := c.newCooldownClient(t)

	events.SetResponseStatusForNextPosts(recoverableEventStatus, 1)
	c.sendArbitraryEvent(t, client)
	client.FlushEvents(t)
	events.Endpoint().RequireConnection(t, defaultEventTimeout) // the failing post

	// A later post is delivered promptly rather than suppressed for a window. This asserts continuation,
	// not the retry mechanic (an SDK may also retry the failed post, which likewise delivers).
	c.sendArbitraryEvent(t, client)
	client.FlushEvents(t)
	events.ExpectAnalyticsEvents(t, defaultEventTimeout)
}

// eventCooldownUnexpectedEntersCooldown: an unexpected failure enters cooldown -- the failed post is not
// retried and an in-window flush is a no-op (EVTPROC Requirements 1.7.3, 1.8.1, 1.8.5). Observed without
// a full window, since an unexpected failure (unlike a recoverable one) produces no retry.
func (c CommonEventTests) eventCooldownUnexpectedEntersCooldown(t *ldtest.T) {
	client, events := c.newCooldownClient(t)

	events.SetResponseStatus(unexpectedEventStatus)
	c.sendArbitraryEvent(t, client)
	client.FlushEvents(t)
	events.Endpoint().RequireConnection(t, defaultEventTimeout) // the failing post

	// No retry: cooldown engaged. The wait also ensures the failure is processed before the next flush,
	// so that flush lands inside the window.
	events.Endpoint().RequireNoMoreConnections(t, noCooldownRetryTimeout)

	// An in-window flush is a no-op.
	c.sendArbitraryEvent(t, client)
	client.FlushEvents(t)
	events.Endpoint().RequireNoMoreConnections(t, noCooldownRetryTimeout)
}

// eventCooldownResumesAfterWindow: the processor resumes after the window rather than stopping for good
// (EVTPROC Requirements 1.8.1-1.8.3, 1.8.6). Waits out a real window, so it is long-running.
func (c CommonEventTests) eventCooldownResumesAfterWindow(t *ldtest.T) {
	t.LongRunning()
	client, events := c.newCooldownClient(t)

	// Enter cooldown.
	events.SetResponseStatus(unexpectedEventStatus)
	c.sendArbitraryEvent(t, client)
	client.FlushEvents(t)
	events.Endpoint().RequireConnection(t, defaultEventTimeout) // the failing post

	// Record (do not flush) an event during the window. Recording is not gated by cooldown, so the
	// machinery keeps running -- but an in-window scheduled flush disposes of whatever it drains
	// exactly as a sent-and-failed delivery would, so a non-durable SDK legitimately discards this
	// event before the window ends. It is recorded anyway because it makes the silence check
	// stronger: a non-conformant SDK that keeps delivering through the window has a non-empty batch
	// to betray it.
	c.sendArbitraryEvent(t, client)

	// Real in-window silence: over an interval guaranteed to lie inside the window, no delivery is made.
	// The failure is still injected, so a non-conformant SDK that retries through the window is caught
	// here. (A violation in the [window/2, window] tail is indistinguishable from a legitimate probe
	// without time compression; Test 3 covers the immediate-retry case.)
	events.Endpoint().RequireNoMoreConnections(t, inWindowSilenceCheck)

	// Accept again, then wait past the longest possible window before probing. Clearing now (not at the
	// probe) keeps a conformant SDK's own window-end probe from hitting the failure and re-cooling.
	// The 45s margin absorbs scheduling delay on loaded runners: the elapsed time is measured from
	// RequireConnection rather than from when the SDK processed the failure.
	events.SetResponseStatus(0)
	time.Sleep(eventCooldownWindow - inWindowSilenceCheck + 45*time.Second)

	// Record a fresh event now that the window has certainly elapsed, so the probe has something to
	// deliver for every SDK: a non-durable processor has discarded its in-window batches (parity with
	// a refusing server), while a durable one may additionally redeliver retained payloads; both
	// conform. Contents are therefore not asserted.
	c.sendArbitraryEvent(t, client)

	// The probe is accepted, proving resumption.
	client.FlushEvents(t)
	events.ExpectAnalyticsEvents(t, defaultEventTimeout)
}
