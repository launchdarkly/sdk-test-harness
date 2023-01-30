package sdktests

import (
	"fmt"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/launchdarkly/sdk-test-harness/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
)

type commonTestsBase struct {
	sdkKind        mockld.SDKKind
	isClientSide   bool
	isMobile       bool
	isPHP          bool
	sdkConfigurers []SDKConfigurer
	userFactory    *UserFactory
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

//// String returns a human-readable representation of the filter key,
//// suitable for test output.
func (p environmentFilter) String() string {
	if !p.IsDefined() {
		return "no environment filter"
	}
	return fmt.Sprintf("environment_filter_key=\"%s\"", p.Value())
}

// Matcher checks that if the filter is present, then the query parameter map contains a parameter
// named "filter" with its value.
// If the filter is not present (envFilterNone), checks that the query parameter map *does not* contain
// a parameter named "filter".
func (p environmentFilter) Matcher() m.Matcher {
	hasFilter := m.MapIncluding(
		m.KV("filter", m.Equal(p.Value())),
	)
	if !p.IsDefined() {
		hasFilter = m.Not(hasFilter)
	}
	return QueryParameters().Should(hasFilter)
}

func newCommonTestsBase(t *ldtest.T, testName string, baseSDKConfigurers ...SDKConfigurer) commonTestsBase {
	c := commonTestsBase{
		sdkKind:     requireContext(t).sdkKind,
		userFactory: NewUserFactory(testName),
	}
	c.isClientSide = c.sdkKind.IsClientSide()
	c.isMobile = t.Capabilities().Has(servicedef.CapabilityMobile)
	c.isPHP = c.sdkKind == mockld.PHPSDK
	if c.isClientSide {
		c.sdkConfigurers = append(
			[]SDKConfigurer{
				WithClientSideConfig(servicedef.SDKConfigClientSideParams{
					InitialUser: c.userFactory.NextUniqueUser(),
				}),
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
		params.User = o.Some(lduser.NewUser("user-key"))
	}
	client.SendCustomEvent(t, params)
}
