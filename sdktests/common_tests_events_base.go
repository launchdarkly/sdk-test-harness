package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

const defaultEventTimeout = time.Second * 5

// CommonEventTests groups together event-related test methods that are shared between server-side and client-side.
type CommonEventTests struct {
	commonTestsBase
}

func NewCommonEventTests(t *ldtest.T, testName string, baseSDKConfigurers ...SDKConfigurer) CommonEventTests {
	return CommonEventTests{newCommonTestsBase(t, testName, baseSDKConfigurers...)}
}

func (c CommonEventTests) discardIdentifyEventIfClientSide(t *ldtest.T, client *SDKClient, events *SDKEventSink) {
	if c.isClientSide {
		client.FlushEvents(t)
		payload := events.ExpectAnalyticsEvents(t, time.Second)
		m.In(t).Assert(payload, m.Items(IsIdentifyEvent()))
		// Also drain the connection record for this initial identify post. ExpectAnalyticsEvents only
		// consumes the payload; the endpoint records the connection separately (and before the payload),
		// so without this the stale connection would offset every later RequireConnection /
		// RequireNoMoreConnections assertion by one. The connection is already enqueued by the time the
		// payload arrives, so this returns immediately.
		events.Endpoint().RequireConnection(t, time.Second)
	}
}

func (c CommonEventTests) initialEventPayloadExpectations() []m.Matcher {
	// Server-side SDKs do not send any events in the first payload unless some action are taken
	if !c.isClientSide {
		return nil
	}
	// Client-side SDKs always send an initial identify event
	return []m.Matcher{IsIdentifyEvent()}
}

//nolint:unused // May not be used now, but could be helpful in new tests.
func (c CommonEventTests) eventsWithIndexEventIfAppropriate(matchers ...m.Matcher) []m.Matcher {
	// Server-side SDKs (excluding PHP) send an index event for each never-before-seen user. Client-side
	// SDKs and the PHP SDK do not.
	if c.isClientSide || c.isPHP {
		return matchers
	}
	return append([]m.Matcher{IsIndexEvent()}, matchers...)
}

//nolint:unused // May not be used now, but could be helpful in new tests.
func (c CommonEventTests) eventsWithIndexEventAndSummaryEventIfAppropriate(matchers ...m.Matcher) []m.Matcher {
	return c.eventsWithSummaryEventIfAppropriate(
		c.eventsWithIndexEventIfAppropriate(matchers...)...,
	)
}

//nolint:unused // May not be used now, but could be helpful in new tests.
func (c CommonEventTests) eventsWithSummaryEventIfAppropriate(matchers ...m.Matcher) []m.Matcher {
	// The PHP SDK is the only one that never sends a summary event.
	if c.isPHP {
		return matchers
	}
	return append(append([]m.Matcher(nil), matchers...), IsSummaryEvent())
}
