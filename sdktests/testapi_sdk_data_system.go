package sdktests

import (
	"errors"
	"net/http"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

type sdkDataSystemConfig struct {
	polling             o.Maybe[bool] // true, false, or "undefined, use the default"
	pollingInitializers []mockld.FDv2SDKData
}

// SDKDataSystemOption is the interface for options to NewSDKDataSystem.
type SDKDataSystemOption helpers.ConfigOption[sdkDataSystemConfig]

// DataSystemOptionPollingInitializer adds support for a polling initializer
func DataSystemOptionPollingInitializer(data mockld.FDv2SDKData) SDKDataSystemOption {
	return helpers.ConfigOptionFunc[sdkDataSystemConfig](func(c *sdkDataSystemConfig) error {
		c.pollingInitializers = append(c.pollingInitializers, data)
		return nil
	})
}

// DataSystemOptionPolling makes an SDKDataSystem simulate the polling service.
func DataSystemOptionPolling() SDKDataSystemOption {
	return helpers.ConfigOptionFunc[sdkDataSystemConfig](func(c *sdkDataSystemConfig) error {
		c.polling = o.Some(true)
		return nil
	})
}

// DataSystemOptionStreaming makes an SDKDataSystem simulate the streaming service.
func DataSystemOptionStreaming() SDKDataSystemOption {
	return helpers.ConfigOptionFunc[sdkDataSystemConfig](func(c *sdkDataSystemConfig) error {
		c.polling = o.Some(false)
		return nil
	})
}

type SDKDataSystem struct {
	t             *ldtest.T
	Initializers  []DataInitializer
	Synchronizers []DataSynchronizer
}

func (s *SDKDataSystem) AddDataInitializer(initializer DataInitializer) {
	s.Initializers = append(s.Initializers, initializer)
}

func (s *SDKDataSystem) AddDataSynchronizer(synchronizer DataSynchronizer) {
	s.Synchronizers = append(s.Synchronizers, synchronizer)
}

func (s *SDKDataSystem) PrimarySync() *DataSynchronizer {
	return &s.Synchronizers[0]
}

// Configure updates the SDK client configuration for NewSDKClient, causing the SDK
// to connect to the appropriate base URI for the data source test fixture. This only works if
// the data source was created along with its own endpoint, with NewSDKDataSystem; if it was
// created as a handler to be used in a separately configured endpoint, you have to set the
// base URI in the test logic rather than using this shortcut.
func (s *SDKDataSystem) Configure(config *servicedef.SDKConfigParams) error {
	if len(s.Initializers) == 0 && len(s.Synchronizers) == 0 {
		return errors.New("tried to use an SDKDataSystem with no initializers or synchronizers")
	}

	dataSystem := config.DataSystem.OrElse(servicedef.DataSystem{})
	if len(s.Initializers) > 0 {
		initializers := []servicedef.DataInitializer{}

		for _, initializer := range s.Initializers {
			if initializer.pollingService == nil {
				continue
			}

			if initializer.endpoint == nil {
				initializer.endpoint =
					requireContext(s.t).harness.NewMockEndpoint(
						initializer.pollingService,
						s.t.DebugLogger(),
						harness.MockEndpointDescription("polling initializer"))
				s.t.Defer(initializer.endpoint.Close)
			}

			initializers = append(initializers, servicedef.DataInitializer{
				Polling: o.Some(servicedef.SDKConfigPollingParams{
					BaseURI: initializer.endpoint.BaseURL(),
				}),
			})
		}
		dataSystem.Initializers = initializers
	}

	if len(s.Synchronizers) > 0 {
		synchronizers := make([]servicedef.DataSynchronizer, len(s.Synchronizers))

		for i := range s.Synchronizers {
			sync := &s.Synchronizers[i]
			if sync.streaming != nil {
				if sync.endpoint == nil {
					sync.endpoint =
						requireContext(s.t).harness.NewMockEndpoint(
							sync.streaming,
							s.t.DebugLogger(),
							harness.MockEndpointDescription("streaming synchronizer"))
					s.t.Defer(sync.endpoint.Close)
				}
				synchronizers[i].Streaming = o.Some(servicedef.SDKConfigStreamingParams{
					BaseURI: sync.endpoint.BaseURL(),
				})
			} else if sync.polling != nil {
				if sync.endpoint == nil {
					sync.endpoint =
						requireContext(s.t).harness.NewMockEndpoint(
							sync.polling,
							s.t.DebugLogger(),
							harness.MockEndpointDescription("polling synchronizer"))
					s.t.Defer(sync.endpoint.Close)
				}
				synchronizers[i].Polling = o.Some(servicedef.SDKConfigPollingParams{
					BaseURI: sync.endpoint.BaseURL(),
				})
			}
		}

		dataSystem.Synchronizers = synchronizers
	}

	config.DataSystem = o.Some(dataSystem)

	return nil
}

type DataInitializer struct {
	pollingService *mockld.PollingService
	endpoint       *harness.MockEndpoint
}

func (d *DataInitializer) Endpoint() *harness.MockEndpoint { return d.endpoint }

type DataSynchronizer struct {
	streaming *mockld.StreamingService
	polling   *mockld.PollingService
	endpoint  *harness.MockEndpoint
}

func (d *DataSynchronizer) Endpoint() *harness.MockEndpoint { return d.endpoint }

// NewSDKDataSystem creates a new SDKDataSystem with the specified initial data set.
//
// It can simulate the streaming service or the polling service. If you don't explicitly specify
// DataSystemOptionPolling or DataSystemOptionStreaming, the default depends on what kind of SDK is being
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
func NewSDKDataSystem(
	t *ldtest.T, data mockld.SDKData, options ...SDKDataSystemOption) *SDKDataSystem {
	dataSystem := NewSDKDataSystemWithoutEndpoints(t, data, options...)

	if dataSystem.Initializers != nil {
		for i, initializer := range dataSystem.Initializers {
			if initializer.pollingService == nil {
				continue
			}

			initializer.endpoint =
				requireContext(t).harness.NewMockEndpoint(initializer.pollingService, t.DebugLogger(),
					harness.MockEndpointDescription("polling initializer"))

			dataSystem.Initializers[i] = initializer
		}
	}

	for i := range dataSystem.Synchronizers {
		sync := &dataSystem.Synchronizers[i]
		isPolling := sync.polling != nil
		handler := helpers.IfElse[http.Handler](isPolling, sync.polling, sync.streaming)
		sync.endpoint = requireContext(t).harness.NewMockEndpoint(handler, t.DebugLogger(),
			harness.MockEndpointDescription("synchronizer service"))
	}

	return dataSystem
}

// NewSDKDataSystemWithoutEndpoints is the same as NewSDKDataSystem, but it does not allocate an
// endpoint to accept incoming requests. Use this if you want to configure the endpoint separately,
// for instance if you want it to delegate some requests to the data source but return an error
// for some other requests.
func NewSDKDataSystemWithoutEndpoints(
	t *ldtest.T, data mockld.SDKData, options ...SDKDataSystemOption) *SDKDataSystem {
	sdkKind := requireContext(t).sdkKind
	if data == nil {
		data = mockld.EmptyData(sdkKind)
	}

	switch v := data.(type) {
	case mockld.ServerSDKData:
		data = v.ConvertToFDv2SDKData(t)
	case mockld.ClientSDKData:
		data = v.ConvertToFDv2SDKClientData(t, "initial")
	default:
		// no-op, for other data types
	}

	var config sdkDataSystemConfig
	_ = helpers.ApplyOptions(&config, options...)

	d := &SDKDataSystem{}
	d.t = t

	for _, initializer := range config.pollingInitializers {
		d.Initializers = append(d.Initializers, DataInitializer{
			pollingService: mockld.NewPollingService(initializer, sdkKind, t.DebugLogger()).
				WithGzipCompression(t.Capabilities().Has(servicedef.CapabilityPollingGzip)),
		})
	}

	defaultIsPolling := sdkKind == mockld.JSClientSDK || sdkKind == mockld.PHPSDK
	if config.polling.Value() || (!config.polling.IsDefined() && defaultIsPolling) {
		d.AddDataSynchronizer(DataSynchronizer{
			polling: mockld.NewPollingService(data, sdkKind, t.DebugLogger()).
				WithGzipCompression(t.Capabilities().Has(servicedef.CapabilityPollingGzip)),
		})
	} else {
		d.AddDataSynchronizer(DataSynchronizer{
			streaming: mockld.NewStreamingService(data, sdkKind, t.DebugLogger()),
		})
	}

	return d
}
