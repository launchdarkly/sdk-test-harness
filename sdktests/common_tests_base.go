package sdktests

import (
	"fmt"
	"strings"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/launchdarkly/sdk-test-harness/v2/data"
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

type transportProtocol struct {
	name       string
	configurer SDKConfigurer
}

func https(endpoint string) string {
	return strings.Replace(endpoint, "http", "https", 1)
}

func (c commonTestsBase) availableTransports(t *ldtest.T) []transportProtocol {
	configurers := []transportProtocol{
		{"http", NoopConfigurer{}},
	}
	if t.Capabilities().Has(servicedef.CapabilityTLS) {
		configurer := helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(configOut *servicedef.SDKConfigParams) error {
			configOut.TLS = o.Some(servicedef.SDKConfigTLSParams{
				VerifyPeer: false,
			})
			configOut.Streaming = configOut.Streaming.Map(func(p servicedef.SDKConfigStreamingParams) servicedef.SDKConfigStreamingParams {
				p.BaseURI = https(p.BaseURI)
				return p
			})
			configOut.Polling = configOut.Polling.Map(func(p servicedef.SDKConfigPollingParams) servicedef.SDKConfigPollingParams {
				p.BaseURI = https(p.BaseURI)
				return p
			})
			configOut.Events = configOut.Events.Map(func(p servicedef.SDKConfigEventParams) servicedef.SDKConfigEventParams {
				p.BaseURI = https(p.BaseURI)
				return p
			})
			return nil
		})
		configurers = append(configurers, transportProtocol{"https", configurer})
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
