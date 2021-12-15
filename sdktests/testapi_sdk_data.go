package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"
)

// SDKDataSource is a test fixture that provides a callback endpoint for SDK clients to connect to,
// simulating the LaunchDarkly streaming or polling service.
type SDKDataSource struct {
	streamingEndpoint *harness.MockEndpoint
}

// NewSDKDataSource creates a new SDKDataSource with the specified initial data set.
//
// The object's lifecycle is tied to the test scope that created it; it will be automatically closed
// when this test scope exits. It can be reused by subtests until then. Debug output related to the
// data source will be attached to this test scope.
func NewSDKDataSource(t *ldtest.T, data mockld.SDKData) *SDKDataSource {
	ss := mockld.NewStreamingService(defaultSDKKey, data, t.DebugLogger())
	streamingEndpoint := requireContext(t).harness.NewMockEndpoint(ss, nil, t.DebugLogger())
	t.Defer(streamingEndpoint.Close)

	t.Debug("setting SDK data to: %s", string(data.Serialize()))

	return &SDKDataSource{streamingEndpoint: streamingEndpoint}
}

// ApplyConfiguration updates the SDK client configuration for NewSDKClient, causing the SDK
// to connect to the appropriate base URI for the test fixture.
func (d *SDKDataSource) ApplyConfiguration(config *servicedef.SDKConfigParams) {
	if config.Streaming == nil {
		config.Streaming = &servicedef.SDKConfigStreamingParams{}
	} else {
		sc := *config.Streaming
		config.Streaming = &sc // copy to avoid side effects
	}
	config.Streaming.BaseURI = d.streamingEndpoint.BaseURL()
}
