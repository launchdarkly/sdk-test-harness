package sdktests

import (
	"fmt"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

// commonTestsBase provides shared behavior for server-side and client-side SDK tests, if their
// behavior is similar enough to share most of the test logic. Each subcategory of tests defines
// its own type embedding this struct (such as CommonEventTests) so that its methods can be
// namespaced within that category.
//
// When we call newCommonTestsBase, it automatically determines whether this is a client-side or
// a server-side SDK by looking up the test service capabilities. If it is a client-side SDK,
// isClientSide is set to true, and sdkConfigurers is set to include the minimal required
// configuration for a client-side SDK (that is, an initial user). For this to work, the test
// logic should always use baseSDKConfigurationPlus() when creating a client.
type commonTestsBase struct {
	sdkKind        mockld.SDKKind
	isClientSide   bool
	isMobile       bool
	isPHP          bool
	sdkConfigurers []SDKConfigurer
	contextFactory *data.ContextFactory
}

type flagRequestMethod string

const (
	flagRequestGET    flagRequestMethod = "GET"
	flagRequestREPORT flagRequestMethod = "REPORT"
)

// Represents a key identifying a filtered environment. This key is passed into
// SDKs when configuring the polling or streaming data source, and should be
// appended to the end of streaming/polling requests as a URL query parameter
// named "filter".
//
// Example: "foo" -> "?filter=foo"
type environmentFilter struct {
	o.Maybe[string]
}

// String returns a human-readable representation of the filter key,
// suitable for test output.
func (p environmentFilter) String() string {
	return fmt.Sprintf("environment_filter_key=\"%s\"", p.Value())
}

// Matcher checks that if the filter is present, the query parameter map contains a parameter
// named "filter" with its value.
// If the filter is not present, it checks that the query parameter map *does not* contain
// a parameter named "filter".
func (p environmentFilter) Matcher() m.Matcher {
	hasFilter := m.MapIncluding(
		m.KV("filter", m.Equal(p.Value())),
	)
	if !p.IsDefined() {
		hasFilter = m.Not(hasFilter)
	}
	return UniqueQueryParameters().Should(hasFilter)
}

func newCommonTestsBase(t *ldtest.T, testName string, baseSDKConfigurers ...SDKConfigurer) commonTestsBase {
	c := commonTestsBase{
		sdkKind:        requireContext(t).sdkKind,
		contextFactory: data.NewContextFactory(testName),
	}
	c.isClientSide = c.sdkKind.IsClientSide()
	c.isMobile = t.Capabilities().Has(servicedef.CapabilityMobile)
	c.isPHP = c.sdkKind == mockld.PHPSDK
	if c.isClientSide {
		c.sdkConfigurers = append(
			[]SDKConfigurer{
				WithClientSideInitialContext(c.contextFactory.NextUniqueContext()),
			},
			baseSDKConfigurers...,
		)
	} else {
		c.sdkConfigurers = baseSDKConfigurers
	}
	return c
}
func (c commonTestsBase) baseSDKConfigurationPlus(configurers ...SDKConfigurer) []SDKConfigurer {
	return append(c.sdkConfigurers, configurers...)
}

func (c commonTestsBase) authorizationHeaderMatcher(credential string) m.Matcher {
	if c.sdkKind == mockld.JSClientSDK {
		return HasNoAuthorizationHeader()
	}
	return HasAuthorizationHeader(credential)
}

func (c commonTestsBase) availableFlagRequestMethods() []flagRequestMethod {
	if c.isClientSide {
		return []flagRequestMethod{flagRequestGET, flagRequestREPORT}
	}
	return []flagRequestMethod{flagRequestGET}
}

// transportProtocol represents the protocol used to communicate between the test harness and service under test:
// either http or https. This allows SDKs to exercise their TLS stacks, which is required for production usage.
type transportProtocol struct {
	// Either http or https.
	protocol string
	// A function that configures the SDK's TLS options.
	configurer SDKConfigurer
}

// Run invokes T.Run() with the protocol's name, passing in a modified T that is suitable for the test.
func (t transportProtocol) Run(tester *ldtest.T, action func(*ldtest.T)) {
	// This is a pretty nasty hack. We're modifying the TestHarness that is stashed away in T, in order
	// to tell it to use HTTPS when creating mock endpoints. This is necessary because higher level
	// test components - like the mock data sources or event sink - use those methods in their own setup.
	// So, if this is a test that should use HTTPS, tweak the global TestHarness and enable it - then undo
	// it after the test runs. WARNING: this won't work with tests that run in parallel.

	// Ensure that if some test fails/panics, we disable HTTPS for the next one.
	defer requireContext(tester).harness.SetHttps(false)

	tester.Run(t.protocol, func(tester *ldtest.T) {
		requireContext(tester).harness.SetHttps(t.protocol == "https")
		action(tester)
	})
}

// Returns an https transport configuration. Use the Run method to run a test, and pass the configurer to the
// SDK client to set up its TLS configuration.
func (c commonTestsBase) httpsTransport(t *ldtest.T) []transportProtocol {
	var transports []transportProtocol
	if !t.Capabilities().Has(servicedef.CapabilityTLS) {
		return transports
	}
	configurer := helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(configOut *servicedef.SDKConfigParams) error {
		configOut.TLS = o.Some(servicedef.SDKConfigTLSParams{
			// VerifyPeer must be false because the certificate is self-signed and there is no way to configure
			// the SDK with a root of trust. This may be added in the future.
			VerifyPeer: false,
		})
		return nil
	})
	transports = append(transports, transportProtocol{"https", configurer})
	return transports
}

// Returns the transports available for testing. For each transportProtocol returned, use the Run method
// to run a test. Within the test, mock endpoints will be configured as http or https automatically.
// Additionally, pass the transportProtocol's configurer into the SDK client config to properly setup its
// TLS options.
func (c commonTestsBase) availableTransports(t *ldtest.T) []transportProtocol {
	// By default, tests are set up with http. Therefore, there's no need to specifically reconfigure the SDK.
	// If that changes in the future, this would need to be modified.
	configurers := []transportProtocol{
		{"http", NoopConfigurer{}},
	}
	configurers = append(configurers, c.httpsTransport(t)...)
	return configurers
}

func (c commonTestsBase) withVerifyPeer(verifyPeer bool) SDKConfigurer {
	return helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(configOut *servicedef.SDKConfigParams) error {
		configOut.TLS = o.Some(servicedef.SDKConfigTLSParams{
			VerifyPeer: verifyPeer,
		})
		return nil
	})
}

// Returns a set of environment filters for testing, along with a filter representing
// "no filter".
func (c commonTestsBase) environmentFilters() []environmentFilter {
	return []environmentFilter{
		{o.None[string]()},
		{o.Some("encoding_not_necessary")},
		{o.Some("encoding necessary +! %& ( )")},
	}
}

func (c commonTestsBase) withFlagRequestMethod(method flagRequestMethod) SDKConfigurer {
	if !c.isClientSide || (method != flagRequestREPORT) {
		return helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(configOut *servicedef.SDKConfigParams) error {
			return nil
		})
	}
	return helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(configOut *servicedef.SDKConfigParams) error {
		clientSideConfig := configOut.ClientSide.Value()
		clientSideConfig.UseReport = o.Some(true)
		configOut.ClientSide = o.Some(clientSideConfig)
		return nil
	})
}

func (c commonTestsBase) sendArbitraryEvent(t *ldtest.T, client *SDKClient) {
	params := servicedef.CustomEventParams{EventKey: "arbitrary-event"}
	if !c.isClientSide {
		params.Context = o.Some(ldcontext.New("user-key"))
	}
	client.SendCustomEvent(t, params)
}
