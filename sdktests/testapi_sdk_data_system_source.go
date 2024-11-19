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

type sdkDataSystemSourceConfig struct {
	polling o.Maybe[bool] // true, false, or "undefined, use the default"
}

// SDKDataSystemSourceOption is the interface for options to NewSDKDataSystemSource.
type SDKDataSystemSourceOption helpers.ConfigOption[sdkDataSystemSourceConfig]

// DataSystemSourceOptionPolling makes an SDKDataSystemSource simulate the polling service.
func DataSystemSourceOptionPolling() SDKDataSystemSourceOption {
	return helpers.ConfigOptionFunc[sdkDataSystemSourceConfig](func(c *sdkDataSystemSourceConfig) error {
		c.polling = o.Some(true)
		return nil
	})
}

// DataSystemSourceOptionStreaming makes an SDKDataSystemSource simulate the streaming service.
func DataSystemSourceOptionStreaming() SDKDataSystemSourceOption {
	return helpers.ConfigOptionFunc[sdkDataSystemSourceConfig](func(c *sdkDataSystemSourceConfig) error {
		c.polling = o.Some(false)
		return nil
	})
}

type SDKDataSystemSource struct {
	t             *ldtest.T
	Initializers  []DataInitializer
	Synchronizers *Synchronizers
}

func (s *SDKDataSystemSource) AddDataInitializer(initializer DataInitializer) {
	s.Initializers = append(s.Initializers, initializer)
}

func (s *SDKDataSystemSource) SetPrimarySynchronizer(synchronizer Synchronizer) {
	if s.Synchronizers == nil {
		s.Synchronizers = &Synchronizers{}
	}
	s.Synchronizers.primary = synchronizer
}

func (s *SDKDataSystemSource) SetSecondarySynchronizer(synchronizer Synchronizer) {
	if s.Synchronizers == nil {
		s.Synchronizers = &Synchronizers{}
	}
	s.Synchronizers.secondary = &synchronizer
}

// Configure updates the SDK client configuration for NewSDKClient, causing the SDK
// to connect to the appropriate base URI for the data source test fixture. This only works if
// the data source was created along with its own endpoint, with NewSDKDataSystemSource; if it was
// created as a handler to be used in a separately configured endpoint, you have to set the
// base URI in the test logic rather than using this shortcut.
func (s *SDKDataSystemSource) Configure(config *servicedef.SDKConfigParams) error {
	if len(s.Initializers) == 0 && s.Synchronizers == nil {
		return errors.New("tried to use an SDKDataSystemSource with no initializers or synchronizers")
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

	if s.Synchronizers != nil {
		synchronizers := dataSystem.Synchronizers.OrElse(servicedef.Synchronizers{})
		if s.Synchronizers.primary.streamingService != nil {
			streaming := synchronizers.Primary.Streaming.Value()

			if s.Synchronizers.primary.endpoint == nil {
				s.Synchronizers.primary.endpoint =
					requireContext(s.t).harness.NewMockEndpoint(
						s.Synchronizers.primary.streamingService,
						s.t.DebugLogger(),
						harness.MockEndpointDescription("streaming initializer"))
				s.t.Defer(s.Synchronizers.primary.endpoint.Close)
			}
			streaming.BaseURI = s.Synchronizers.primary.endpoint.BaseURL()
			synchronizers.Primary.Streaming = o.Some(streaming)
		} else if s.Synchronizers.primary.pollingService != nil {
			polling := synchronizers.Primary.Polling.Value()

			if s.Synchronizers.primary.endpoint == nil {
				s.Synchronizers.primary.endpoint =
					requireContext(s.t).harness.NewMockEndpoint(
						s.Synchronizers.primary.pollingService,
						s.t.DebugLogger(),
						harness.MockEndpointDescription("polling initializer"))
				s.t.Defer(s.Synchronizers.primary.endpoint.Close)
			}
			polling.BaseURI = s.Synchronizers.primary.endpoint.BaseURL()
			synchronizers.Primary.Polling = o.Some(polling)
		}

		if s.Synchronizers.secondary != nil {
			if s.Synchronizers.secondary.streamingService != nil {
				secondary := synchronizers.Secondary.Value()
				streaming := synchronizers.Secondary.Value().Streaming.Value()

				if s.Synchronizers.secondary.endpoint == nil {
					s.Synchronizers.secondary.endpoint =
						requireContext(s.t).harness.NewMockEndpoint(
							s.Synchronizers.secondary.streamingService,
							s.t.DebugLogger(),
							harness.MockEndpointDescription("streaming synchronizer"))
					s.t.Defer(s.Synchronizers.secondary.endpoint.Close)
				}

				secondary.Streaming = o.Some(streaming)
				synchronizers.Secondary = o.Some(secondary)
			} else if s.Synchronizers.secondary.pollingService != nil {
				secondary := synchronizers.Secondary.Value()
				polling := synchronizers.Secondary.Value().Polling.Value()

				if s.Synchronizers.secondary.endpoint == nil {
					s.Synchronizers.secondary.endpoint =
						requireContext(s.t).harness.NewMockEndpoint(
							s.Synchronizers.secondary.pollingService,
							s.t.DebugLogger(),
							harness.MockEndpointDescription("polling synchronizer"))
					s.t.Defer(s.Synchronizers.secondary.endpoint.Close)
				}

				secondary.Polling = o.Some(polling)
				synchronizers.Secondary = o.Some(secondary)
			}
		}

		dataSystem.Synchronizers = o.Some(synchronizers)
	}

	config.DataSystem = o.Some(dataSystem)

	return nil
}

type DataInitializer struct {
	pollingService *mockld.PollingService
	endpoint       *harness.MockEndpoint
}

func (d *DataInitializer) Endpoint() *harness.MockEndpoint { return d.endpoint }

type Synchronizers struct {
	primary   Synchronizer
	secondary *Synchronizer
}

type Synchronizer struct {
	streamingService *mockld.StreamingService
	pollingService   *mockld.PollingService
	endpoint         *harness.MockEndpoint
}

func (d *Synchronizer) Endpoint() *harness.MockEndpoint { return d.endpoint }

// NewSDKDataSystemSource creates a new SDKDataSystemSource with the specified initial data set.
//
// It can simulate the streaming service or the polling service. If you don't explicitly specify
// DataSystemSourceOptionPolling or DataSystemSourceOptionStreaming, the default depends on what kind of SDK is being
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
func NewSDKDataSystemSource(t *ldtest.T, data mockld.SDKData, options ...SDKDataSystemSourceOption) *SDKDataSystemSource {
	dataSystem := NewSDKDataSystemSourceWithoutEndpoints(t, data, options...)

	if dataSystem.Synchronizers != nil {
		isPolling := dataSystem.Synchronizers.primary.pollingService != nil
		handler := helpers.IfElse[http.Handler](isPolling, dataSystem.Synchronizers.primary.pollingService, dataSystem.Synchronizers.primary.streamingService)
		dataSystem.Synchronizers.primary.endpoint =
			requireContext(t).harness.NewMockEndpoint(handler, t.DebugLogger(),
				harness.MockEndpointDescription("streaming service"))

		if dataSystem.Synchronizers.secondary != nil {
			isPolling := dataSystem.Synchronizers.secondary.pollingService != nil
			handler := helpers.IfElse[http.Handler](isPolling, dataSystem.Synchronizers.secondary.pollingService, dataSystem.Synchronizers.secondary.streamingService)
			dataSystem.Synchronizers.secondary.endpoint =
				requireContext(t).harness.NewMockEndpoint(handler, t.DebugLogger(),
					harness.MockEndpointDescription("streaming service"))
		}
	}

	return dataSystem
}

// NewSDKDataSystemSourceWithoutEndpoint is the same as NewSDKDataSystemSource, but it does not allocate an
// endpoint to accept incoming requests. Use this if you want to configure the endpoint separately,
// for instance if you want it to delegate some requests to the data source but return an error
// for some other requests.
func NewSDKDataSystemSourceWithoutEndpoints(t *ldtest.T, data mockld.SDKData, options ...SDKDataSystemSourceOption) *SDKDataSystemSource {
	sdkKind := requireContext(t).sdkKind
	if data == nil {
		data = mockld.EmptyData(sdkKind)
	}

	if d, ok := data.(mockld.ServerSDKData); ok {
		data = d.ConvertToFDv2SDKData(t)
	}

	var config sdkDataSystemSourceConfig
	_ = helpers.ApplyOptions(&config, options...)

	defaultIsPolling := sdkKind == mockld.JSClientSDK || sdkKind == mockld.PHPSDK
	d := &SDKDataSystemSource{}
	if config.polling.Value() || (!config.polling.IsDefined() && defaultIsPolling) {
		d.Synchronizers = &Synchronizers{
			primary: Synchronizer{
				pollingService: mockld.NewPollingService(data, sdkKind, t.DebugLogger()).WithGzipCompression(t.Capabilities().Has(servicedef.CapabilityPollingGzip)),
			},
		}
	} else {
		d.Synchronizers = &Synchronizers{
			primary: Synchronizer{
				streamingService: mockld.NewStreamingService(data, sdkKind, t.DebugLogger()),
			},
		}
	}

	return d
}
