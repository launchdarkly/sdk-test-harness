package sdktests

import (
	"net/http"

	"github.com/launchdarkly/sdk-test-harness/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"
)

// SDKDataSource is a test fixture that provides a callback endpoint for SDK clients to connect to,
// simulating the LaunchDarkly streaming or polling service.
type SDKDataSource struct {
	streamingService *mockld.StreamingService
	pollingService   *mockld.PollingService
	endpoint         *harness.MockEndpoint
}

// SDKDataSourceOption is the interface for options to NewSDKDataSource.
type SDKDataSourceOption interface {
	isDataSourceOption() // arbitrary method - this is just a marker interface currently
}

type dataSourceOptionPolling struct{}

func (o dataSourceOptionPolling) isDataSourceOption() {}

// DataSourceOptionPolling makes an SDKDataSource simulate the polling service instead of the streaming service.
var DataSourceOptionPolling dataSourceOptionPolling //nolint:gochecknoglobals

// NewSDKDataSource creates a new SDKDataSource with the specified initial data set. By default, it
// is a streaming data source. For polling mode, add the DataSourceOptionPolling option.
//
// The object's lifecycle is tied to the test scope that created it; it will be automatically closed
// when this test scope exits. It can be reused by subtests until then. Debug output related to the
// data source will be attached to this test scope.
func NewSDKDataSource(t *ldtest.T, data mockld.SDKData, options ...SDKDataSourceOption) *SDKDataSource {
	d := NewSDKDataSourceWithoutEndpoint(t, data, options...)

	var handler http.Handler
	var description string
	if d.pollingService != nil {
		handler = d.pollingService
		description = "polling service"
	} else {
		handler = d.streamingService
		description = "streaming service"
	}
	d.endpoint = requireContext(t).harness.NewMockEndpoint(handler, t.DebugLogger(),
		harness.MockEndpointDescription(description))
	t.Defer(d.endpoint.Close)

	return d
}

// NewSDKDataSourceWithoutEndpoint is the same as NewSDKDataSource, but it does not allocate an
// endpoint to accept incoming requests. Use this if you want to configure the endpoint separately,
// for instance if you want it to delegate some requests to the data source but return an error
// for some other requests.
func NewSDKDataSourceWithoutEndpoint(t *ldtest.T, data mockld.SDKData, options ...SDKDataSourceOption) *SDKDataSource {
	isPolling := false
	for _, o := range options {
		if o == DataSourceOptionPolling {
			isPolling = true
		}
	}

	d := &SDKDataSource{}
	if isPolling {
		d.pollingService = mockld.NewPollingService(data, t.DebugLogger())
	} else {
		d.streamingService = mockld.NewStreamingService(data, t.DebugLogger())
	}

	t.Debug("setting SDK data to: %s", string(data.Serialize()))

	return d
}

// Endpoint returns the low-level object that manages incoming requests.
func (d *SDKDataSource) Endpoint() *harness.MockEndpoint { return d.endpoint }

// StreamingService returns the low-level object that manages the stream data, or nil if this is a
// polling data source.
func (d *SDKDataSource) StreamingService() *mockld.StreamingService { return d.streamingService }

// PollingService returns the low-level object that manages the polling data, or nil if this is a
// streaming data source.
func (d *SDKDataSource) PollingService() *mockld.PollingService { return d.pollingService }

// Handler returns the HTTP handler for the service. Since StreamingService implements http.Handler
// already, this is the same as Service() but makes the purpose clearer.
func (d *SDKDataSource) Handler() http.Handler { return d.streamingService }

// ApplyConfiguration updates the SDK client configuration for NewSDKClient, causing the SDK
// to connect to the appropriate base URI for the data source test fixture. This only works if
// the data source was created along with its own endpoint, with NewSDKDataSource; if it was
// created as a handler to be used in a separately configured endpoint, you have to set the
// base URI in the test logic rather than using this shortcut.
func (d *SDKDataSource) ApplyConfiguration(config *servicedef.SDKConfigParams) {
	if d.endpoint == nil {
		panic("Tried to use an SDKDataSource without its own endpoint as a parameter to NewSDKClient")
	}
	if d.streamingService != nil {
		newState := config.Streaming.Value()
		newState.BaseURI = d.endpoint.BaseURL()
		config.Streaming = o.Some(newState)
	}
	if d.pollingService != nil {
		newState := config.Polling.Value()
		newState.BaseURI = d.endpoint.BaseURL()
		config.Polling = o.Some(newState)
	}
}
