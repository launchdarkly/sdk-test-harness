package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/sdktests/expect"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SDKEventSink is a test fixture that provides a callback endpoint for SDK clients to send event data to,
// simulating the LaunchDarkly event-recorder service.
type SDKEventSink struct {
	eventsService  *mockld.EventsService
	eventsEndpoint *harness.MockEndpoint
}

// NewSDKEventSink creates a new SDKEventSink.
//
// The object's lifecycle is tied to the test scope that created it; it will be automatically closed
// when this test scope exits. It can be reused by subtests until then. Debug output related to the event
// sink will be attached to this test scope.
func NewSDKEventSink(t *ldtest.T) *SDKEventSink {
	eventsService := mockld.NewEventsService(requireContext(t).sdkKind, defaultSDKKey, t.DebugLogger())
	eventsEndpoint := requireContext(t).harness.NewMockEndpoint(eventsService, nil, t.DebugLogger())

	t.Defer(eventsEndpoint.Close)

	return &SDKEventSink{
		eventsService:  eventsService,
		eventsEndpoint: eventsEndpoint,
	}
}

// ApplyConfiguration updates the SDK client configuration for NewSDKClient, causing the SDK
// to connect to the appropriate base URI for the test fixture.
func (e *SDKEventSink) ApplyConfiguration(config *servicedef.SDKConfigParams) {
	if config.Events == nil {
		ec := *config.Events
		config.Events = &ec // copy to avoid side effects
	}
	config.Events.BaseURI = e.eventsEndpoint.BaseURL()
}

// AwaitAnalyticsEventPayload waits for event data to be posted to the endpoint. If no new events
// arrive before the timeout, the test immediately fails and terminates.
func (e *SDKEventSink) AwaitAnalyticsEventPayload(t require.TestingT, timeout time.Duration) mockld.Events {
	events, ok := e.eventsService.AwaitAnalyticsEventPayload(timeout)
	if !ok {
		require.Fail(t, "timed out waiting for events")
	}
	return events
}

// ExpectAnalyticsEvents waits for event data to be posted to the endpoint, and then verifies the
// specified expectations. If no new events arrive before the timeout, the test immediately fails
// and terminates.
//
// The number of events posted must be the same as the number of expectations.
func (e *SDKEventSink) ExpectAnalyticsEvents(
	t require.TestingT,
	timeout time.Duration,
	eventMatchers ...expect.EventExpectation,
) {
	events := e.AwaitAnalyticsEventPayload(t, timeout)
	if len(events) != len(eventMatchers) {
		require.Fail(t, "received wrong number of events", "expected %d events, got: %s",
			len(eventMatchers), events.JSONString())
	}
	ok := true
	for i, m := range eventMatchers {
		if !m.For(t, events[i]) {
			ok = false
		}
	}
	if !ok {
		assert.Fail(t, "at least one event expectation failed")
	}
}

// ExpectNoAnalyticsEvents waits for the specified timeout and fails if any events are posted before then.
func (e *SDKEventSink) ExpectNoAnalyticsEvents(t require.TestingT, timeout time.Duration) {
	events, ok := e.eventsService.AwaitAnalyticsEventPayload(timeout)
	if ok {
		require.Fail(t, "received events when none were expected", "events: %s", events.JSONString())
	}
}
