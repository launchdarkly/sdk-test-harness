package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"

	"github.com/stretchr/testify/require"
)

func baseEventsConfig() servicedef.SDKConfigEventParams {
	return servicedef.SDKConfigEventParams{
		// Set a very long flush interval so event payloads will only be flushed when we force a flush
		FlushIntervalMS: o.Some(ldtime.UnixMillisecondTime(1000000)),
	}
}

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
	eventsEndpoint := requireContext(t).harness.NewMockEndpoint(eventsService, t.DebugLogger(),
		harness.MockEndpointDescription("events service"))

	t.Defer(eventsEndpoint.Close)

	return &SDKEventSink{
		eventsService:  eventsService,
		eventsEndpoint: eventsEndpoint,
	}
}

// ApplyConfiguration updates the SDK client configuration for NewSDKClient, causing the SDK
// to connect to the appropriate base URI for the test fixture.
func (e *SDKEventSink) ApplyConfiguration(config *servicedef.SDKConfigParams) {
	newState := config.Events.Value()
	newState.BaseURI = e.eventsEndpoint.BaseURL()
	config.Events = o.Some(newState)
}

// Endpoint returns the low-level object that manages incoming requests.
func (e *SDKEventSink) Endpoint() *harness.MockEndpoint { return e.eventsEndpoint }

// Service returns the underlying mock events service component, for access to special options.
func (e *SDKEventSink) Service() *mockld.EventsService { return e.eventsService }

// ExpectAnalyticsEvents waits for event data to be posted to the endpoint, and then calls
// matchers.ItemsInAnyOrder with the specified eventMatchers, verifying that the payload contains
// one event matching each of the matchers regardless of ordering.
//
// If no new events arrive before the timeout, the test immediately fails and terminates.
//
// The number of events posted must be the same as the number of matchers.
func (e *SDKEventSink) ExpectAnalyticsEvents(t require.TestingT, timeout time.Duration) mockld.Events {
	events, ok := e.eventsService.AwaitAnalyticsEventPayload(timeout)
	if !ok {
		require.Fail(t, "timed out waiting for events")
	}
	return events
}

// ExpectNoAnalyticsEvents waits for the specified timeout and fails if any events are posted before then.
func (e *SDKEventSink) ExpectNoAnalyticsEvents(t require.TestingT, timeout time.Duration) {
	events, ok := e.eventsService.AwaitAnalyticsEventPayload(timeout)
	if ok {
		require.Fail(t, "received events when none were expected", "events: %s", events.JSONString())
	}
}
