package sdktests

import (
	"net/http"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"

	"github.com/stretchr/testify/require"
)

type connectionModeEntry struct {
	name    string
	options []SDKDataSystemOption
}

type sdkDataSystemConfig struct {
	polling                 o.Maybe[bool]
	pollingInitializers     []mockld.SDKData
	pollingSynchronizerOpts []DataSynchronizerOption
	connectionModes         []connectionModeEntry
	initialConnectionMode   string
	environmentID           o.Maybe[string]
}

// SDKDataSystemOption is the interface for options to NewSDKDataSystem.
type SDKDataSystemOption helpers.ConfigOption[sdkDataSystemConfig]

// DataSystemOptionPollingInitializer adds a polling initializer that serves the given data.
func DataSystemOptionPollingInitializer(data mockld.SDKData) SDKDataSystemOption {
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

// DataSystemOptionPollInterval sets the polling interval (pollIntervalMs) for the default polling
// synchronizer. Only applies when the data system is in polling mode. Omit to use the SDK's default.
// The interval is stored on the synchronizer (per-sync); this option applies to the single polling
// sync created by NewSDKDataSystem/NewSDKDataSystemWithoutEndpoints.
func DataSystemOptionPollInterval(interval time.Duration) SDKDataSystemOption {
	return helpers.ConfigOptionFunc[sdkDataSystemConfig](func(c *sdkDataSystemConfig) error {
		c.pollingSynchronizerOpts = append(c.pollingSynchronizerOpts, SynchronizerOptionPollInterval(interval))
		return nil
	})
}

// DataSystemOptionEnvironmentID makes every mock service of the data system report an environment
// ID in the X-LD-EnvID response header, as LaunchDarkly does. It must be specified at the top level;
// it applies to all connection modes.
func DataSystemOptionEnvironmentID(environmentID string) SDKDataSystemOption {
	return helpers.ConfigOptionFunc[sdkDataSystemConfig](func(c *sdkDataSystemConfig) error {
		c.environmentID = o.Some(environmentID)
		return nil
	})
}

// DataSystemOptionConnectionMode defines a named connection mode with its own set of
// initializers and synchronizers. The inner options (e.g. DataSystemOptionPolling,
// DataSystemOptionStreaming) control what mock services the mode contains. If no inner
// options are given, the mode uses the SDK-kind default (polling for JS, streaming for
// mobile/server).
//
// When any connection mode is defined, Configure will emit connectionModeConfig instead
// of top-level initializers/synchronizers.
func DataSystemOptionConnectionMode(name string, options ...SDKDataSystemOption) SDKDataSystemOption {
	return helpers.ConfigOptionFunc[sdkDataSystemConfig](func(c *sdkDataSystemConfig) error {
		c.connectionModes = append(c.connectionModes, connectionModeEntry{name: name, options: options})
		return nil
	})
}

// DataSystemOptionInitialConnectionMode sets which connection mode the SDK should start in.
func DataSystemOptionInitialConnectionMode(name string) SDKDataSystemOption {
	return helpers.ConfigOptionFunc[sdkDataSystemConfig](func(c *sdkDataSystemConfig) error {
		c.initialConnectionMode = name
		return nil
	})
}

type dataSystemMode struct {
	initializers  []DataInitializer
	synchronizers []DataSynchronizer
}

// SDKDataSystem is a test fixture that simulates the LaunchDarkly services that deliver flag data to the
// SDK, and configures the SDK to connect to them.
//
// It uses the data protocol of the current branch of the test tree. In the fdv2 branch, it has the
// initializers and synchronizers of an FDv2 data system, and its Configure method sets the "dataSystem"
// property of the SDK configuration. In the fdv1 branch, its only synchronizer is the FDv1 streaming or
// polling service, and its Configure method sets the top-level "streaming" and "polling" properties. A
// client-side SDK that uses FDv1 streaming can also have one polling initializer, which is the polling
// service that it gets its initial data from.
type SDKDataSystem struct {
	t                     *ldtest.T
	protocol              mockld.DataProtocol
	Initializers          []DataInitializer
	Synchronizers         []DataSynchronizer
	connectionModes       map[string]*dataSystemMode
	initialConnectionMode string
	environmentID         o.Maybe[string]
}

// ConnectionMode returns the named connection mode, or nil if it doesn't exist.
func (s *SDKDataSystem) ConnectionMode(name string) *dataSystemMode {
	return s.connectionModes[name]
}

func (s *SDKDataSystem) AddDataInitializer(initializer DataInitializer) {
	s.Initializers = append(s.Initializers, initializer)
}

func (s *SDKDataSystem) AddDataSynchronizer(synchronizer DataSynchronizer) {
	s.Synchronizers = append(s.Synchronizers, synchronizer)
}

// SetData updates all mock services (initializers and synchronizers across all connection modes)
// with new data. The data is automatically converted to FDv2 format if needed. Use this in
// general-purpose tests that need the mock environment to reflect updated flag state without
// caring about endpoint-specific behavior.
func (s *SDKDataSystem) SetData(data mockld.SDKData) {
	data = convertData(s.t, data)
	setDataOnServices(s.Initializers, s.Synchronizers, data)
	for _, mode := range s.connectionModes {
		setDataOnServices(mode.initializers, mode.synchronizers, data)
	}
}

// buildServiceDefComponents converts DataInitializer/DataSynchronizer slices into their
// servicedef equivalents, lazily creating endpoints for any that don't have one yet.
func (s *SDKDataSystem) buildServiceDefComponents(
	initializers []DataInitializer,
	synchronizers []DataSynchronizer,
) ([]servicedef.DataInitializer, []servicedef.DataSynchronizer) {
	var sdInitializers []servicedef.DataInitializer
	for i := range initializers {
		init := &initializers[i]
		if init.pollingService == nil {
			continue
		}
		if init.endpoint == nil {
			init.endpoint =
				requireContext(s.t).harness.NewMockEndpoint(
					init.pollingService,
					s.t.DebugLogger(),
					harness.MockEndpointDescription("polling initializer"))
			s.t.Defer(init.endpoint.Close)
		}
		sdInitializers = append(sdInitializers, servicedef.DataInitializer{
			Polling: o.Some(servicedef.SDKConfigPollingParams{
				BaseURI: init.endpoint.BaseURL(),
			}),
		})
	}

	var sdSynchronizers []servicedef.DataSynchronizer
	if len(synchronizers) > 0 {
		sdSynchronizers = make([]servicedef.DataSynchronizer, len(synchronizers))
		for i := range synchronizers {
			sync := &synchronizers[i]
			if sync.streaming != nil {
				if sync.endpoint == nil {
					sync.endpoint =
						requireContext(s.t).harness.NewMockEndpoint(
							sync.streaming,
							s.t.DebugLogger(),
							harness.MockEndpointDescription("streaming synchronizer"))
					s.t.Defer(sync.endpoint.Close)
				}
				sdSynchronizers[i].Streaming = o.Some(servicedef.SDKConfigStreamingParams{
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
				params := servicedef.SDKConfigPollingParams{BaseURI: sync.endpoint.BaseURL()}
				if sync.pollIntervalMS.IsDefined() {
					ms := uint64(sync.pollIntervalMS.Value().Milliseconds()) //nolint:gosec
					params.PollIntervalMS = o.Some(ldtime.UnixMillisecondTime(ms))
				}
				sdSynchronizers[i].Polling = o.Some(params)
			}
		}
	}

	return sdInitializers, sdSynchronizers
}

// Configure updates the SDK client configuration for NewSDKClient. If connection modes are
// defined, it emits config.DataSystem.ConnectionModeConfig; otherwise it emits top-level
// config.DataSystem.Initializers/Synchronizers.
func (s *SDKDataSystem) Configure(config *servicedef.SDKConfigParams) error {
	if s.protocol == mockld.FDv1 {
		return s.configureFDv1(config)
	}
	if len(s.connectionModes) > 0 {
		return s.configureWithConnectionModes(config)
	}
	return s.configureTopLevel(config)
}

func (s *SDKDataSystem) configureTopLevel(config *servicedef.SDKConfigParams) error {
	initializers, synchronizers := s.buildServiceDefComponents(s.Initializers, s.Synchronizers)

	dataSystem := config.DataSystem.OrElse(servicedef.DataSystem{})
	if len(initializers) > 0 {
		dataSystem.Initializers = initializers
	}
	if len(synchronizers) > 0 {
		dataSystem.Synchronizers = synchronizers
	}
	config.DataSystem = o.Some(dataSystem)

	return nil
}

// configureFDv1 sets the base URIs of the FDv1 streaming and polling services, and the polling interval.
// It keeps the other streaming and polling properties that a previous configurer set.
func (s *SDKDataSystem) configureFDv1(config *servicedef.SDKConfigParams) error {
	initializers, synchronizers := s.buildServiceDefComponents(s.Initializers, s.Synchronizers)
	for _, initializer := range initializers {
		polling := config.Polling.Value()
		polling.BaseURI = initializer.Polling.Value().BaseURI
		config.Polling = o.Some(polling)
	}
	for _, sync := range synchronizers {
		if sync.Streaming.IsDefined() {
			streaming := config.Streaming.Value()
			streaming.BaseURI = sync.Streaming.Value().BaseURI
			config.Streaming = o.Some(streaming)
		}
		if sync.Polling.IsDefined() {
			polling := config.Polling.Value()
			polling.BaseURI = sync.Polling.Value().BaseURI
			if sync.Polling.Value().PollIntervalMS.IsDefined() {
				polling.PollIntervalMS = sync.Polling.Value().PollIntervalMS
			}
			config.Polling = o.Some(polling)
		}
	}
	return nil
}

func (s *SDKDataSystem) configureWithConnectionModes(config *servicedef.SDKConfigParams) error {
	ds := config.DataSystem.OrElse(servicedef.DataSystem{})
	connMode := ds.ConnectionModeConfig.OrElse(servicedef.ConnectionModeConfig{})
	modes := connMode.CustomConnectionModes.OrElse(map[string]servicedef.ModeDefinition{})

	for modeName, mode := range s.connectionModes {
		initializers, synchronizers := s.buildServiceDefComponents(mode.initializers, mode.synchronizers)
		modes[modeName] = servicedef.ModeDefinition{
			Initializers:  initializers,
			Synchronizers: synchronizers,
		}
	}

	connMode.CustomConnectionModes = o.Some(modes)
	connMode.InitialConnectionMode = o.Some(s.initialConnectionMode)
	ds.ConnectionModeConfig = o.Some(connMode)
	config.DataSystem = o.Some(ds)

	return nil
}

// WithConnectionModeSynchronizer returns an SDKConfigurer that adds a single synchronizer
// to a named connection mode entry. Use this for tests that need to override or append
// synchronizer URIs outside the data system (e.g. trailing-slash or proxy URI tests).
func WithConnectionModeSynchronizer(modeName string, sync servicedef.DataSynchronizer) SDKConfigurer {
	return helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(config *servicedef.SDKConfigParams) error {
		ds := config.DataSystem.OrElse(servicedef.DataSystem{})
		connMode := ds.ConnectionModeConfig.OrElse(servicedef.ConnectionModeConfig{})
		modes := connMode.CustomConnectionModes.OrElse(map[string]servicedef.ModeDefinition{})

		existing := modes[modeName]
		existing.Synchronizers = append(existing.Synchronizers, sync)
		modes[modeName] = existing

		connMode.CustomConnectionModes = o.Some(modes)
		ds.ConnectionModeConfig = o.Some(connMode)
		config.DataSystem = o.Some(ds)
		return nil
	})
}

// WithInitialConnectionMode returns an SDKConfigurer that sets the initial connection mode
// in config.DataSystem.ConnectionModeConfig.
func WithInitialConnectionMode(modeName string) SDKConfigurer {
	return helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(config *servicedef.SDKConfigParams) error {
		ds := config.DataSystem.OrElse(servicedef.DataSystem{})
		connMode := ds.ConnectionModeConfig.OrElse(servicedef.ConnectionModeConfig{})
		connMode.InitialConnectionMode = o.Some(modeName)
		ds.ConnectionModeConfig = o.Some(connMode)
		config.DataSystem = o.Some(ds)
		return nil
	})
}

type DataInitializer struct {
	pollingService *mockld.PollingService
	endpoint       *harness.MockEndpoint
}

func (d *DataInitializer) Endpoint() *harness.MockEndpoint { return d.endpoint }

type DataSynchronizer struct {
	streaming      *mockld.StreamingService
	polling        *mockld.PollingService
	endpoint       *harness.MockEndpoint
	pollIntervalMS o.Maybe[time.Duration] // if set, sent to SDK as pollIntervalMs for this sync
}

func (d *DataSynchronizer) Endpoint() *harness.MockEndpoint { return d.endpoint }

// DataSynchronizerOption modifies a DataSynchronizer (e.g. poll interval for a polling sync).
// Options are applied when a synchronizer is created; multiple polling syncs can each have their own options.
type DataSynchronizerOption func(*DataSynchronizer)

// SynchronizerOptionPollInterval sets pollIntervalMs for this synchronizer. Only meaningful for polling syncs.
func SynchronizerOptionPollInterval(interval time.Duration) DataSynchronizerOption {
	return func(d *DataSynchronizer) {
		d.pollIntervalMS = o.Some(interval)
	}
}

// NewSDKDataSystem creates an SDKDataSystem with sensible defaults and allocates mock
// endpoints. This is the standard entry point for most tests.
//
// Caller-supplied options are applied after the defaults and can override them (e.g. a
// connection mode with the same name as a default mode replaces it).
//
// It automatically detects (from the ldtest.T properties) whether we are testing a server-side,
// mobile, or JS-based client-side SDK, and configures the endpoint behavior as appropriate. The
// endpoints will enforce that the client only uses supported URL paths and HTTP methods; however,
// they do not do any validation of credentials (SDK key, mobile key, environment ID) since that
// would require this component to know more about the overall configuration than it knows. We
// have specific tests that do verify that the SDKs send appropriate credentials.
//
// The object's lifecycle is tied to the test scope that created it; it will be automatically
// closed when this test scope exits. It can be reused by subtests until then. Debug output
// related to the data source will be attached to this test scope.
func NewSDKDataSystem(
	t *ldtest.T, data mockld.SDKData, options ...SDKDataSystemOption) *SDKDataSystem {
	dataSystem := NewSDKDataSystemWithoutEndpoints(t, data, options...)
	dataSystem.CreateEndpoints()
	return dataSystem
}

// NewSDKDataSystemWithoutEndpoints is the same as NewSDKDataSystem (sensible defaults) but
// does not allocate endpoints. Use this when you need to configure endpoints separately,
// for instance to compose handlers into a sequential handler for retry/reconnection tests.
func NewSDKDataSystemWithoutEndpoints(
	t *ldtest.T, data mockld.SDKData, options ...SDKDataSystemOption) *SDKDataSystem {
	data = convertData(t, data)
	defaults := defaultDataSystemOptions(t, data)
	combined := make([]SDKDataSystemOption, 0, len(defaults)+len(options))
	combined = append(combined, defaults...)
	combined = append(combined, options...)
	return buildDataSystem(t, data, combined)
}

// NewSDKDataSystemCustom creates an SDKDataSystem with no implicit defaults. The caller
// must explicitly specify initializers, synchronizers, and/or connection modes via options.
// No endpoints are allocated; call CreateEndpoints afterwards if needed.
//
// Use this for tests that need precise control over the data pipeline, such as FDv2 tests
// that set different data for initializers vs. synchronizers.
func NewSDKDataSystemCustom(
	t *ldtest.T, data mockld.SDKData, options ...SDKDataSystemOption) *SDKDataSystem {
	data = convertData(t, data)
	return buildDataSystem(t, data, options)
}

// CreateEndpoints allocates mock endpoints for all initializers and synchronizers in this
// data system. Call this after NewSDKDataSystemCustom when endpoints are needed.
func (s *SDKDataSystem) CreateEndpoints() {
	if len(s.connectionModes) > 0 {
		for _, mode := range s.connectionModes {
			createEndpoints(s.t, mode.initializers, mode.synchronizers, s.environmentID)
		}
	} else {
		createEndpoints(s.t, s.Initializers, s.Synchronizers, s.environmentID)
	}
}

// convertData converts SDKData to FDv2 format if needed and fills in empty data defaults.
func convertData(t *ldtest.T, data mockld.SDKData) mockld.SDKData {
	sdkKind := requireContext(t).sdkKind
	if data == nil {
		data = mockld.EmptyData(sdkKind)
	}
	switch v := data.(type) {
	case mockld.ServerSDKData:
		return v.ConvertToFDv2SDKData(t)
	case mockld.ClientSDKData:
		return v.ConvertToFDv2SDKClientData(t, "initial")
	default:
		return data
	}
}

// defaultDataSystemOptions returns the implicit defaults for the SDK kind. The converted
// data is used to create a polling initializer so the SDK can receive data with a selector
// during initialization.
//
// In FDv2, client-side SDKs get a "streaming" connection mode with a polling initializer and streaming
// synchronizer. Server-side SDKs get a top-level polling initializer and synchronizer
// (streaming for most, polling for PHP). In FDv1, SDKs get a single streaming or polling service.
func defaultDataSystemOptions(t *ldtest.T, data mockld.SDKData) []SDKDataSystemOption {
	sdkKind := requireContext(t).sdkKind
	if dataProtocol(t) == mockld.FDv1 {
		// FDv1 SDKs have no initializers. JS-based client-side SDKs and the PHP SDK poll by default, and the
		// other SDKs stream by default.
		if sdkKind == mockld.JSClientSDK || sdkKind == mockld.PHPSDK {
			return []SDKDataSystemOption{DataSystemOptionPolling()}
		}
		return []SDKDataSystemOption{DataSystemOptionStreaming()}
	}
	if sdkKind.IsClientSide() {
		return []SDKDataSystemOption{
			DataSystemOptionConnectionMode("streaming",
				DataSystemOptionPollingInitializer(data),
				DataSystemOptionStreaming(),
			),
			DataSystemOptionInitialConnectionMode("streaming"),
		}
	}
	opts := []SDKDataSystemOption{DataSystemOptionPollingInitializer(data)}
	if sdkKind == mockld.PHPSDK {
		opts = append(opts, DataSystemOptionPolling())
	} else {
		opts = append(opts, DataSystemOptionStreaming())
	}
	return opts
}

// buildDataSystem is the internal constructor shared by all NewSDKDataSystem variants.
// It expects already-converted data and processes options without adding any implicit defaults.
func buildDataSystem(t *ldtest.T, data mockld.SDKData, options []SDKDataSystemOption) *SDKDataSystem {
	sdkKind := requireContext(t).sdkKind
	protocol := dataProtocol(t)

	var config sdkDataSystemConfig
	_ = helpers.ApplyOptions(&config, options...)

	d := &SDKDataSystem{t: t, protocol: protocol, environmentID: config.environmentID}

	switch {
	case protocol == mockld.FDv1 && len(config.connectionModes) > 0:
		// FDv1 has no connection modes. The initial connection mode supplies the data services.
		mode := findConnectionMode(t, config)
		built := buildMode(t, data, sdkKind, mode.options)
		d.Initializers = built.initializers
		d.Synchronizers = built.synchronizers
	case len(config.connectionModes) > 0:
		d.connectionModes = make(map[string]*dataSystemMode)
		d.initialConnectionMode = config.initialConnectionMode

		for _, modeEntry := range config.connectionModes {
			mode := buildMode(t, data, sdkKind, modeEntry.options)
			d.connectionModes[modeEntry.name] = mode
		}

		if mode, ok := d.connectionModes[d.initialConnectionMode]; ok {
			d.Synchronizers = mode.synchronizers
			d.Initializers = mode.initializers
		}
	default:
		for _, initializer := range config.pollingInitializers {
			d.Initializers = append(d.Initializers, DataInitializer{
				pollingService: newMockPollingService(t, convertData(t, initializer)),
			})
		}

		if config.polling.IsDefined() {
			if config.polling.Value() {
				sync := DataSynchronizer{
					polling: newMockPollingService(t, data),
				}
				for _, opt := range config.pollingSynchronizerOpts {
					opt(&sync)
				}
				d.AddDataSynchronizer(sync)
			} else {
				d.AddDataSynchronizer(DataSynchronizer{
					streaming: newMockStreamingService(t, data),
				})
			}
		}
	}

	if protocol == mockld.FDv1 {
		requireFDv1DataServices(t, d)
	}
	return d
}

// findConnectionMode returns the initial connection mode of the configuration. It fails the test if
// there is no such mode.
func findConnectionMode(t *ldtest.T, config sdkDataSystemConfig) connectionModeEntry {
	for _, mode := range config.connectionModes {
		if mode.name == config.initialConnectionMode {
			return mode
		}
	}
	require.Failf(t, "test logic error", "the data system has no initial connection mode %q",
		config.initialConnectionMode)
	return connectionModeEntry{}
}

// requireFDv1DataServices fails the test unless an FDv1 SDK can use the data services. An FDv1 SDK has one
// streaming or polling service. A client-side SDK that streams can also get its initial data from one
// polling service.
func requireFDv1DataServices(t *ldtest.T, d *SDKDataSystem) {
	switch {
	case len(d.Synchronizers) > 1:
		require.Fail(t, "test logic error", "an FDv1 SDK cannot have more than one synchronizer")
	case len(d.Initializers) == 0:
		return
	case len(d.Initializers) > 1 || requireContext(t).sdkKind.IsServerSide() ||
		len(d.Synchronizers) == 0 || d.Synchronizers[0].streaming == nil:
		require.Fail(t, "test logic error",
			"an FDv1 SDK can have a polling initializer only if it is a client-side SDK that streams")
	}
}

// newMockStreamingService creates a mock streaming service that uses the data protocol of the test.
func newMockStreamingService(t *ldtest.T, data mockld.SDKData) *mockld.StreamingService {
	return mockld.NewStreamingServiceForProtocol(dataProtocol(t), data, requireContext(t).sdkKind, t.DebugLogger())
}

// newMockPollingService creates a mock polling service that uses the data protocol of the test.
func newMockPollingService(t *ldtest.T, data mockld.SDKData) *mockld.PollingService {
	return mockld.NewPollingServiceForProtocol(dataProtocol(t), data, requireContext(t).sdkKind, t.DebugLogger()).
		WithGzipCompression(t.Capabilities().Has(servicedef.CapabilityPollingGzip))
}

func createEndpoints(
	t *ldtest.T,
	initializers []DataInitializer,
	synchronizers []DataSynchronizer,
	environmentID o.Maybe[string],
) {
	for i := range initializers {
		init := &initializers[i]
		if init.pollingService == nil {
			continue
		}
		init.endpoint =
			requireContext(t).harness.NewMockEndpoint(
				withEnvironmentIDHeader(init.pollingService, environmentID), t.DebugLogger(),
				harness.MockEndpointDescription("polling initializer"))
		t.Defer(init.endpoint.Close)
	}

	for i := range synchronizers {
		sync := &synchronizers[i]
		isPolling := sync.polling != nil
		handler := helpers.IfElse[http.Handler](isPolling, sync.polling, sync.streaming)
		sync.endpoint = requireContext(t).harness.NewMockEndpoint(
			withEnvironmentIDHeader(handler, environmentID), t.DebugLogger(),
			harness.MockEndpointDescription("synchronizer service"))
		t.Defer(sync.endpoint.Close)
	}
}

// withEnvironmentIDHeader wraps a handler so that its responses carry the X-LD-EnvID header.
// If no environment ID is defined, the handler is returned unchanged.
func withEnvironmentIDHeader(handler http.Handler, environmentID o.Maybe[string]) http.Handler {
	if !environmentID.IsDefined() {
		return handler
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(environmentIDHeader, environmentID.Value())
		handler.ServeHTTP(w, r)
	})
}

func setDataOnServices(initializers []DataInitializer, synchronizers []DataSynchronizer, data mockld.SDKData) {
	for _, init := range initializers {
		if init.pollingService != nil {
			init.pollingService.SetData(data)
		}
	}
	for _, sync := range synchronizers {
		if sync.streaming != nil {
			sync.streaming.SetInitialData(data)
		}
		if sync.polling != nil {
			sync.polling.SetData(data)
		}
	}
}

func buildMode(
	t *ldtest.T, data mockld.SDKData, sdkKind mockld.SDKKind, options []SDKDataSystemOption,
) *dataSystemMode {
	var modeConfig sdkDataSystemConfig
	_ = helpers.ApplyOptions(&modeConfig, options...)

	mode := &dataSystemMode{}

	for _, initData := range modeConfig.pollingInitializers {
		mode.initializers = append(mode.initializers, DataInitializer{
			pollingService: newMockPollingService(t, convertData(t, initData)),
		})
	}

	defaultIsPolling := sdkKind == mockld.JSClientSDK || sdkKind == mockld.PHPSDK
	if modeConfig.polling.Value() || (!modeConfig.polling.IsDefined() && defaultIsPolling) {
		sync := DataSynchronizer{
			polling: newMockPollingService(t, data),
		}
		for _, opt := range modeConfig.pollingSynchronizerOpts {
			opt(&sync)
		}
		mode.synchronizers = append(mode.synchronizers, sync)
	} else {
		mode.synchronizers = append(mode.synchronizers, DataSynchronizer{
			streaming: newMockStreamingService(t, data),
		})
	}

	return mode
}
