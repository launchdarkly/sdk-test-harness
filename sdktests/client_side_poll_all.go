package sdktests

import (
	"strings"

	h "github.com/launchdarkly/sdk-test-harness/v3/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

func doClientSidePollTests(t *ldtest.T) {
	t.Run("requests", doClientSidePollRequestTest)
	t.Run("interval", func(t *ldtest.T) {
		doPollIntervalTests(t, WithCredential("my-sdk-key"))
	})
}

func doClientSidePollRequestTest(t *ldtest.T) {
	sdkKind := requireContext(t).sdkKind
	envIDOrMobileKey := "my-credential"

	pollTests := NewCommonPollingTests(t, "doClientSidePollRequestTest",
		WithCredential(envIDOrMobileKey))

	pollTests.RequestMethodAndHeaders(t, envIDOrMobileKey)
	if t.Capabilities().Has(servicedef.CapabilityETagCaching) {
		pollTests.InitialRequestIncludesCorrectEtag(t)
	}

	if t.Capabilities().Has(servicedef.CapabilityHTTPProxy) {
		pollTests.RequestViaHTTPProxy(t)
	}

	requestPathMatcher := func(method flagRequestMethod) m.Matcher {
		switch sdkKind {
		case mockld.MobileSDK, mockld.JSClientSDK:
			getPathPrefix := strings.TrimSuffix(
				mockld.PollingPathFDv2ClientGet,
				mockld.PollingPathContextBase64Param, // details of base64-encoded context data are tested separately
			)
			return h.IfElse(method.sendsContextInBody(),
				m.Equal(mockld.PollingPathFDv2ClientPost),
				m.StringHasPrefix(getPathPrefix))

		case mockld.RokuSDK:
			rokuGetPathPrefix := strings.TrimSuffix(mockld.PollingPathMobileGet, mockld.PollingPathContextBase64Param)
			return h.IfElse(method == flagRequestREPORT,
				m.Equal(mockld.PollingPathMobileReport),
				m.StringHasPrefix(rokuGetPathPrefix))

		default:
			panic("invalid SDK kind")
		}
	}
	pollTests.RequestURLPath(t, requestPathMatcher)

	getPath := h.IfElse(sdkKind == mockld.RokuSDK,
		mockld.PollingPathMobileGet,
		mockld.PollingPathFDv2ClientGet)
	pollTests.RequestContextProperties(t, getPath)
}
