package sdktests

import (
	"errors"
	"net/http"

	"github.com/launchdarkly/sdk-test-harness/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/framework/helpers"
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

type sdkDataSourceConfig struct {
	polling o.Maybe[bool] // true, false, or "undefined, use the default"
}

// SDKDataSourceOption is the interface for options to NewSDKDataSource.
type SDKDataSourceOption helpers.ConfigOption[sdkDataSourceConfig]

// DataSourceOptionPolling makes an SDKDataSource simulate the polling service.
func DataSourceOptionPolling() SDKDataSourceOption {
	return helpers.ConfigOptionFunc[sdkDataSourceConfig](func(c *sdkDataSourceConfig) error {
		c.polling = o.Some(true)
		return nil
	})
}

// DataSourceOptionStreaming makes an SDKDataSource simulate the streaming service.
func DataSourceOptionStreaming() SDKDataSourceOption {
	return helpers.ConfigOptionFunc[sdkDataSourceConfig](func(c *sdkDataSourceConfig) error {
		c.polling = o.Some(false)
		return nil
	})
}

// NewSDKDataSource creates a new SDKDataSource with the specified initial data set.
//
// It can simulate either the streaming service or the polling service. If you don't explicitly specify
// DataSourceOptionPolling or DataSourceOptionStreaming, the default depends on what kind of SDK is being
// tested: server-side and mobile SDKs default to streaming, JS-based client-side SDKs default to polling.
//
// It automatically detects (from the ldtest.T properties) whether we are testing a server-side, mobile,
// or JS-based client-side SDK, and configures the endpoint behavior as appropriate. The endpoints will
// enforce that the client only uses supported URL paths and HTTP methods; however, they do not do any
// validation of credentials (SDK key, mobile key, environment ID) since that would require this component
// to know more about the overall configuration than it knows. We have specific tests that do verify that
// the SDKs send appropriate credentials.
//
// The object's lifecycle is tied to the test scope that created it; it will be automatically closed
// when this test scope exits. It can be reused by subtests until then. Debug output related to the
// data source will be attached to this test scope.
func NewSDKDataSource(t *ldtest.T, data mockld.SDKData, options ...SDKDataSourceOption) *SDKDataSource {
	d := NewSDKDataSourceWithoutEndpoint(t, data, options...)

	isPolling := d.pollingService != nil
	handler := helpers.IfElse[http.Handler](isPolling, d.pollingService, d.streamingService)
	description := helpers.IfElse(isPolling, "polling service", "streaming service")

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
	var config sdkDataSourceConfig
	_ = helpers.ApplyOptions(&config, options...)

	sdkKind := requireContext(t).sdkKind
	if data == nil {
		data = mockld.EmptyData(sdkKind)
	}

	defaultIsPolling := sdkKind == mockld.JSClientSDK
	d := &SDKDataSource{}
	if config.polling.Value() || (!config.polling.IsDefined() && defaultIsPolling) {
		d.pollingService = mockld.NewPollingService(data, sdkKind, t.DebugLogger())
	} else {
		d.streamingService = mockld.NewStreamingService(data, sdkKind, t.DebugLogger())
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

// Configure updates the SDK client configuration for NewSDKClient, causing the SDK
// to connect to the appropriate base URI for the data source test fixture. This only works if
// the data source was created along with its own endpoint, with NewSDKDataSource; if it was
// created as a handler to be used in a separately configured endpoint, you have to set the
// base URI in the test logic rather than using this shortcut.
func (d *SDKDataSource) Configure(config *servicedef.SDKConfigParams) error {
	if d.endpoint == nil {
		return errors.New("tried to use an SDKDataSource without its own endpoint as a parameter to NewSDKClient")
	}
	if d.streamingService == nil && d.pollingService == nil {
		return errors.New("tried to use an SDKDataSource that has neither streaming nor polling configured")
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
	return nil
}
