package sdktests

import (
	"sync/atomic"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v3/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

// RequestsCompactPayloads checks that an SDK asks for compact payloads on its FDv2 streaming
// requests if its compact payloads option is on, and does not ask for them if the option is off.
func (c CommonStreamingTests) RequestsCompactPayloads(t *ldtest.T) {
	for _, option := range compactPayloadsOptionCases {
		t.Run(option.name, func(t *ldtest.T) {
			t.RequireCapability(servicedef.CapabilityCompactPayloads)

			dataSystem, configurers := c.setupDataSystems(t, nil)
			configurers = append(configurers, withCompactPayloadsOption(option.enable))
			_ = NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)

			request := dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)
			m.In(t).For("compact payloads query parameter").
				Assert(request.URL.Query().Get(mockld.CompactPayloadsQueryParam), m.Equal(option.queryValue))
		})
	}
}

// RequestsCompactPayloads checks that an SDK asks for compact payloads on its FDv2 polling
// requests if its compact payloads option is on, and does not ask for them if the option is off.
func (c CommonPollingTests) RequestsCompactPayloads(t *ldtest.T) {
	for _, option := range compactPayloadsOptionCases {
		t.Run(option.name, func(t *ldtest.T) {
			t.RequireCapability(servicedef.CapabilityCompactPayloads)

			dataSystem := NewSDKDataSystem(t, nil, c.pollingDataSystemOptions()...)
			_ = NewSDKClient(t, c.baseSDKConfigurationPlus(dataSystem, withCompactPayloadsOption(option.enable))...)

			request := dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)
			m.In(t).For("compact payloads query parameter").
				Assert(request.URL.Query().Get(mockld.CompactPayloadsQueryParam), m.Equal(option.queryValue))
		})
	}
}

// compactPayloadsOptionCases are the values of the compact payloads option that the request tests
// use, with the value of the query parameter that the SDK must send for each.
var compactPayloadsOptionCases = []struct { //nolint:gochecknoglobals // constant test cases
	name       string
	enable     bool
	queryValue string
}{
	{"requests compact payloads", true, mockld.CompactPayloadsQueryValue},
	{"does not request compact payloads when the option is off", false, ""},
}

// withCompactPayloadsOption sets the compact payloads option in the FDv2 data system of an SDK
// configuration. It must come after the configurer that sets the data system.
func withCompactPayloadsOption(enable bool) SDKConfigurer {
	return helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(config *servicedef.SDKConfigParams) error {
		dataSystem := config.DataSystem.Value()
		dataSystem.CompactPayloads = o.Some(enable)
		config.DataSystem = o.Some(dataSystem)
		return nil
	})
}

// compactPayloadsMode is the value of the compact payloads option that the test harness gives to
// every SDK client with an FDv2 data system. The test harness sets it once, before the tests
// start, from its command line.
var compactPayloadsMode atomic.Bool //nolint:gochecknoglobals // set once for the whole test run

func setCompactPayloadsMode(enable bool) {
	compactPayloadsMode.Store(enable)
}

// setCompactPayloadsOption sets the compact payloads option in an SDK configuration, if the SDK has
// the compact-payloads capability and the configuration has an FDv2 data system. A test that sets
// the option itself keeps its value.
func setCompactPayloadsOption(t *ldtest.T, config *servicedef.SDKConfigParams) {
	if !t.Capabilities().Has(servicedef.CapabilityCompactPayloads) || !config.DataSystem.IsDefined() {
		return
	}
	dataSystem := config.DataSystem.Value()
	if !dataSystem.CompactPayloads.IsDefined() {
		dataSystem.CompactPayloads = o.Some(compactPayloadsMode.Load())
		config.DataSystem = o.Some(dataSystem)
	}
}
