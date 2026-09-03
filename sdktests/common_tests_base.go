package sdktests

import (
	"encoding/json"
	"fmt"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/launchdarkly/sdk-test-harness/v3/data"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"
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
	sdkKind               mockld.SDKKind
	isClientSide          bool
	isMobile              bool
	isPHP                 bool
	sdkConfigurers        []SDKConfigurer
	contextFactory        *data.ContextFactory
	flagEvaluationContext ldcontext.Context
}

type flagRequestMethod string

const (
	flagRequestGET    flagRequestMethod = "GET"
	flagRequestREPORT flagRequestMethod = "REPORT"
	flagRequestPOST   flagRequestMethod = "POST"
)

// sendsContextInBody is true for the request methods that carry the evaluation context in the
// request body rather than base64url-encoded in the URL path.
func (m flagRequestMethod) sendsContextInBody() bool {
	return m != flagRequestGET
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
		ctx := c.contextFactory.NextUniqueContext()
		c.flagEvaluationContext = ctx
		c.sdkConfigurers = append(
			[]SDKConfigurer{WithClientSideInitialContext(ctx)},
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
		return m.AnyOf(HasNoAuthorizationHeader(), HasAuthorizationHeader(credential))
	}
	return HasAuthorizationHeader(credential)
}

func (c commonTestsBase) availableFlagRequestMethods(t *ldtest.T) []flagRequestMethod {
	methods := []flagRequestMethod{flagRequestGET}
	if !c.isClientSide {
		return methods
	}
	if t.Capabilities().Has(servicedef.CapabilityClientUseReport) {
		methods = append(methods, flagRequestREPORT)
	}
	if t.Capabilities().Has(servicedef.CapabilityClientUsePost) {
		methods = append(methods, flagRequestPOST)
	}
	return methods
}

// requireBodyFlagRequestMethod skips the test unless the SDK can send the evaluation context in the
// request body, and returns the method to use for that: REPORT when the SDK declares
// client-use-report, otherwise POST when it declares client-use-post.
func (c commonTestsBase) requireBodyFlagRequestMethod(t *ldtest.T) flagRequestMethod {
	if t.Capabilities().Has(servicedef.CapabilityClientUseReport) {
		return flagRequestREPORT
	}
	if t.Capabilities().Has(servicedef.CapabilityClientUsePost) {
		return flagRequestPOST
	}
	t.SkipWithReason(fmt.Sprintf("test service has neither capability %q nor %q",
		servicedef.CapabilityClientUseReport, servicedef.CapabilityClientUsePost))
	return flagRequestGET // unreachable: SkipWithReason ends the test
}

// transportProtocol represents the protocol used to communicate between the test harness and service under test:
// either http or https. This allows SDKs to exercise their TLS stacks, which is required for production usage.
type transportProtocol struct {
	// Either http or https.
	protocol string
	// Tag appended to the test name
	tag string
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

	// Ensure that if some test fails/panics, we are back to using HTTP by default for the next one.
	defer requireContext(tester).harness.SetService("http")

	tester.Run(t.tag, func(tester *ldtest.T) {
		requireContext(tester).harness.SetService(t.protocol)
		action(tester)
	})
}

// Returns a transportProtocol that runs test under HTTPS.
func (c commonTestsBase) withHTTPSTransport(t *ldtest.T) transportProtocol {
	t.RequireCapability(servicedef.CapabilityTLSVerifyPeer)
	// SDKs must verify peers by default, there's nothing to configure.
	return transportProtocol{"https", "https-verify-peer", NoopConfigurer{}}
}

// Returns a transportProtocol that runs the test under HTTPS with peer verification disabled.
func (c commonTestsBase) withHTTPSTransportSkipVerifyPeer(t *ldtest.T) transportProtocol {
	t.RequireCapability(servicedef.CapabilityTLSSkipVerifyPeer)
	configurer := helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(configOut *servicedef.SDKConfigParams) error {
		configOut.TLS = o.Some(servicedef.SDKConfigTLSParams{
			SkipVerifyPeer: true,
		})
		return nil
	})
	return transportProtocol{"https", "https-skip-verify-peer", configurer}
}

func (c commonTestsBase) withHTTPSTransportVerifyPeerCustomCA(t *ldtest.T, customCAFile string) transportProtocol {
	t.RequireCapabilities(servicedef.CapabilityTLSCustomCA, servicedef.CapabilityTLSVerifyPeer)
	configurer := helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(configOut *servicedef.SDKConfigParams) error {
		configOut.TLS = o.Some(servicedef.SDKConfigTLSParams{
			SkipVerifyPeer: false,
			CustomCAFile:   customCAFile,
		})
		return nil
	})
	return transportProtocol{"https", "https-verify-peer-custom-ca", configurer}
}

// Returns the transports available for testing. For each transportProtocol returned, use the Run method
// to run a test. Within the test, mock endpoints will be configured as http or https automatically.
// Additionally, pass the transportProtocol's configurer into the SDK client config to properly set up its
// TLS options.
func (c commonTestsBase) withAvailableTransports(t *ldtest.T) []transportProtocol {
	// By default, tests are set up with http. Therefore, there's no need to specifically reconfigure the SDK.
	// If that changes in the future, this would need to be modified.
	configurers := []transportProtocol{
		{"http", "http", NoopConfigurer{}},
	}
	if t.Capabilities().Has(servicedef.CapabilityTLSSkipVerifyPeer) {
		configurers = append(configurers, c.withHTTPSTransportSkipVerifyPeer(t))
	}
	if t.Capabilities().HasAll(servicedef.CapabilityTLSCustomCA, servicedef.CapabilityTLSVerifyPeer) {
		configurers = append(configurers, c.withHTTPSTransportVerifyPeerCustomCA(t,
			requireContext(t).harness.CertificateAuthorityFile()))
	}
	return configurers
}

func (c commonTestsBase) withFlagRequestMethod(method flagRequestMethod) SDKConfigurer {
	if !c.isClientSide || !method.sendsContextInBody() {
		return helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(configOut *servicedef.SDKConfigParams) error {
			return nil
		})
	}
	return helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(configOut *servicedef.SDKConfigParams) error {
		clientSideConfig := configOut.ClientSide.Value()
		switch method {
		case flagRequestREPORT:
			clientSideConfig.UseReport = o.Some(true)
		case flagRequestPOST:
			clientSideConfig.UsePost = o.Some(true)
		}
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

// emptyFDv1FallbackBody returns an empty FDv1 polling payload in the format the
// SDK kind expects: server-side SDKs receive a {"flags":..,"segments":..} object,
// while client-side SDKs receive a flat map of flag evaluations. The FDv1 fallback
// directive subtests only need initialization to complete (no flag values) so the
// request header can be asserted, so the payload is empty in either format.
//
// Client-side support for the FDv1 Fallback Directive is in progress; serving the
// correct format per kind keeps these subtests valid for both rather than feeding a
// client-side SDK an unparseable server-side body.
func (c commonTestsBase) emptyFDv1FallbackBody() []byte {
	var body any
	if c.isClientSide {
		body = mockld.ClientSDKData{}
	} else {
		body = map[string]any{
			"flags":    map[string]json.RawMessage{},
			"segments": map[string]json.RawMessage{},
		}
	}
	bytes, err := json.Marshal(body)
	if err != nil {
		panic(fmt.Errorf("failed to marshal empty FDv1 fallback body: %w", err))
	}
	return bytes
}

func (c commonTestsBase) withHTTPProxy(url string) SDKConfigurer {
	return helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(configOut *servicedef.SDKConfigParams) error {
		configOut.Proxy = o.Some(servicedef.SDKConfigProxyParams{
			HTTPProxy: o.Some(url),
		})
		return nil
	})
}
