package sdktests

import (
	"fmt"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
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
	// A function that configures the SDK to use the specified transport, by modifying the base polling/streaming/events
	// URIs to use the selected protocol. It takes a MockEndpoint as a roundabout way of discovering the base URI - this
	// is passed in during the test setup.
	configurer func(dataSource *harness.MockEndpoint, events *harness.MockEndpoint) SDKConfigurer
}

// Configurer returns an SDKConfigurer function that configures the SDK to use the specified transport. The SDK's base
// polling/streaming/event URIs will be determined by inspecting the provided MockEndpoint.
func (t transportProtocol) ConfigurerDataSource(dataSource *harness.MockEndpoint) SDKConfigurer {
	return t.ConfigureDataSourceAndEvents(dataSource, nil)
}

func (t transportProtocol) ConfigureDataSourceAndEvents(dataSource *harness.MockEndpoint, events *harness.MockEndpoint) SDKConfigurer {
	return t.configurer(dataSource, events)
}

// Returns the transports available for testing. The resulting list of transports can then be called to create
// SDKConfigurers which should be passed into the test's SDKClient.
func (c commonTestsBase) availableTransports(t *ldtest.T) []transportProtocol {
	// By default, tests are set up with http. Therefore, there's no need to specifically reconfigure the SDK.
	// If that changes in the future, this would need to be modified.
	configurers := []transportProtocol{
		{"http", func(ep *harness.MockEndpoint, ep2 *harness.MockEndpoint) SDKConfigurer {
			// NoopConfigurer doesn't do anything.
			return NoopConfigurer{}
		}},
	}
	// Only SDKs that are capable of configuring TLS should be tested with https.
	if t.Capabilities().Has(servicedef.CapabilityTLS) {
		outer := func(ds *harness.MockEndpoint, events *harness.MockEndpoint) SDKConfigurer {
			configurer := helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(configOut *servicedef.SDKConfigParams) error {
				configOut.TLS = o.Some(servicedef.SDKConfigTLSParams{
					// VerifyPeer must be false because the certificate is self-signed and there is no way to configure
					// the SDK with a root of trust. This may be added in the future.
					VerifyPeer: false,
				})
				// It's valid to pass in BaseHttpsURL() because at startup, the harness checks the TLS capability and if
				// present, also starts an https server with the same handlers as the default http server.
				// The purpose of changing all three using the .Map function is so that this works for
				// polling/streaming/event tests out of the box, but doesn't set the polling/streaming/event configs if
				// they aren't already present.
				configOut.Streaming = configOut.Streaming.Map(func(p servicedef.SDKConfigStreamingParams) servicedef.SDKConfigStreamingParams {
					p.BaseURI = ds.BaseHttpsURL()
					return p
				})
				configOut.Polling = configOut.Polling.Map(func(p servicedef.SDKConfigPollingParams) servicedef.SDKConfigPollingParams {
					p.BaseURI = ds.BaseHttpsURL()
					return p
				})
				if events != nil {
					configOut.Events = configOut.Events.Map(func(p servicedef.SDKConfigEventParams) servicedef.SDKConfigEventParams {
						p.BaseURI = events.BaseHttpsURL()
						return p
					})
				}
				return nil
			})
			return configurer
		}
		configurers = append(configurers, transportProtocol{"https", outer})
	}
	return configurers
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
