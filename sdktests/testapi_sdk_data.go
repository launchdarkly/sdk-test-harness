package sdktests

import (
	"net/http"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

// SDKDataSource is a test fixture that provides a callback endpoint for SDK clients to connect to,
// simulating the LaunchDarkly streaming or polling service.
type SDKDataSource struct {
	streamingService  *mockld.StreamingService
	streamingEndpoint *harness.MockEndpoint
}

// NewSDKDataSource creates a new SDKDataSource with the specified initial data set.
//
// The object's lifecycle is tied to the test scope that created it; it will be automatically closed
// when this test scope exits. It can be reused by subtests until then. Debug output related to the
// data source will be attached to this test scope.
func NewSDKDataSource(t *ldtest.T, data mockld.SDKData) *SDKDataSource {
	ss := mockld.NewStreamingService(data, t.DebugLogger())
	streamingEndpoint := requireContext(t).harness.NewMockEndpoint(ss, nil, t.DebugLogger())
	t.Defer(streamingEndpoint.Close)

	t.Debug("setting SDK data to: %s", string(data.Serialize()))

	return &SDKDataSource{streamingService: ss, streamingEndpoint: streamingEndpoint}
}

// NewSDKDataSourceWithoutEndpoint is the same as NewSDKDataSource, but it does not allocate an
// endpoint to accept incoming requests. Use this if you want to configure the endpoint separately,
// for instance if you want it to delegate some requests to the data source but return an error
// for some other requests.
func NewSDKDataSourceWithoutEndpoint(t *ldtest.T, data mockld.SDKData) *SDKDataSource {
	ss := mockld.NewStreamingService(data, t.DebugLogger())

	t.Debug("setting SDK data to: %s", string(data.Serialize()))

	return &SDKDataSource{streamingService: ss, streamingEndpoint: nil}
}

// Endpoint returns the low-level object that manages incoming requests.
func (d *SDKDataSource) Endpoint() *harness.MockEndpoint { return d.streamingEndpoint }

// Service returns the low-level object that manages the stream data.
func (d *SDKDataSource) Service() *mockld.StreamingService { return d.streamingService }

// Handler returns the HTTP handler for the stream. Since StreamingService implements http.Handler
// already, this is the same as Service() but makes the purpose clearer.
func (d *SDKDataSource) Handler() http.Handler { return d.streamingService }

// ApplyConfiguration updates the SDK client configuration for NewSDKClient, causing the SDK
// to connect to the appropriate base URI for the data source test fixture. This only works if
// the data source was created along with its own endpoint, with NewSDKDataSource; if it was
// created as a handler to be used in a separately configured endpoint, you have to set the
// base URI in the test logic rather than using this shortcut.
func (d *SDKDataSource) ApplyConfiguration(config *servicedef.SDKConfigParams) {
	if d.streamingEndpoint == nil {
		panic("Tried to use an SDKDataSource without its own endpoint as a parameter to NewSDKClient")
	}
	newState := config.Streaming.Value()
	newState.BaseURI = d.streamingEndpoint.BaseURL()
	config.Streaming = o.Some(newState)
}
