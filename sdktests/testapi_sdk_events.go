package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldtime"

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
// It automatically detects (from the ldtest.T properties) whether we are testing a server-side, mobile,
// or JS-based client-side SDK, and configures the endpoint behavior as appropriate. The endpoints will
// enforce that the client only uses supported URL paths and HTTP methods; however, they do not do any
// validation of credentials (SDK key, mobile key, environment ID) since that would require this component
// to know more about the overall configuration than it knows. We have specific tests that do verify that
// the SDKs send appropriate credentials.
//
// The object's lifecycle is tied to the test scope that created it; it will be automatically closed
// when this test scope exits. It can be reused by subtests until then. Debug output related to the event
// sink will be attached to this test scope, and also to any of its subtests that are active when the
// output is generated.
func NewSDKEventSink(t *ldtest.T) *SDKEventSink {
	eventsService := mockld.NewEventsService(
		requireContext(t).sdkKind,
		t.DebugLogger(),
		t.Capabilities().Has(servicedef.CapabilityEventGzip))
	eventsEndpoint := requireContext(t).harness.NewMockEndpoint(eventsService, t.DebugLogger(),
		harness.MockEndpointDescription("events service"))

	t.Defer(eventsEndpoint.Close)

	return &SDKEventSink{
		eventsService:  eventsService,
		eventsEndpoint: eventsEndpoint,
	}
}

// Configure updates the SDK client configuration for NewSDKClient, causing the SDK
// to connect to the appropriate base URI for the test fixture.
func (e *SDKEventSink) Configure(config *servicedef.SDKConfigParams) error {
	newState := config.Events.Value()
	newState.BaseURI = e.eventsEndpoint.BaseURL()
	config.Events = o.Some(newState)
	return nil
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
