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
	t.Run("retry behavior", doClientSidePollRetryTests)
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

	if dataProtocol(t) == mockld.FDv1 {
		pollTests.RequestURLPath(t, func(method flagRequestMethod) m.Matcher {
			return fdv1ClientPollingPathMatcher(sdkKind, envIDOrMobileKey, method)
		})
		pollTests.RequestContextProperties(t, fdv1ClientPollingGetPath(sdkKind, envIDOrMobileKey))
		return
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

// fdv1ClientPollingPathMatcher matches the path of an FDv1 poll request from a client-side SDK. It does
// not check the base64-encoded context data in the path of a GET request, which other tests check.
func fdv1ClientPollingPathMatcher(sdkKind mockld.SDKKind, envID string, method flagRequestMethod) m.Matcher {
	if sdkKind == mockld.JSClientSDK {
		getPathPrefix := strings.TrimSuffix(
			strings.ReplaceAll(mockld.PollingPathJSClientGet, mockld.PollingPathEnvIDParam, envID),
			mockld.PollingPathContextBase64Param)
		reportPath := strings.ReplaceAll(mockld.PollingPathJSClientReport, mockld.PollingPathEnvIDParam, envID)
		return h.IfElse(method == flagRequestREPORT, m.Equal(reportPath), m.StringHasPrefix(getPathPrefix))
	}
	getPathPrefix := strings.TrimSuffix(mockld.PollingPathMobileGet, mockld.PollingPathContextBase64Param)
	return h.IfElse(method == flagRequestREPORT,
		m.Equal(mockld.PollingPathMobileReport),
		m.StringHasPrefix(getPathPrefix))
}

// fdv1ClientPollingGetPath returns the path template of an FDv1 GET poll request from a client-side SDK.
func fdv1ClientPollingGetPath(sdkKind mockld.SDKKind, envID string) string {
	if sdkKind == mockld.JSClientSDK {
		return strings.ReplaceAll(mockld.PollingPathJSClientGet, mockld.PollingPathEnvIDParam, envID)
	}
	return mockld.PollingPathMobileGet
}
