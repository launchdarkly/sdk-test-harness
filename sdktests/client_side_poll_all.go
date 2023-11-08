package sdktests

import (
	"strings"

	h "github.com/launchdarkly/sdk-test-harness/v3/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

func doClientSidePollTests(t *ldtest.T) {
	t.Run("requests", doClientSidePollRequestTest)
}

func doClientSidePollRequestTest(t *ldtest.T) {
	sdkKind := requireContext(t).sdkKind
	envIDOrMobileKey := "my-credential"

	pollTests := NewCommonPollingTests(t, "doClientSidePollRequestTest",
		WithCredential(envIDOrMobileKey))

	pollTests.RequestMethodAndHeaders(t, envIDOrMobileKey)

	requestPathMatcher := func(method flagRequestMethod) m.Matcher {
		switch sdkKind {
		case mockld.RokuSDK:
			fallthrough
		case mockld.MobileSDK:
			mobileGetPathPrefix := strings.TrimSuffix(mockld.PollingPathMobileGet, mockld.PollingPathContextBase64Param)
			return h.IfElse(method == flagRequestREPORT,
				m.Equal(mockld.PollingPathMobileReport),
				m.StringHasPrefix(mobileGetPathPrefix))
			// details of base64-encoded context data are tested separately

		case mockld.JSClientSDK:
			jsGetPathPrefix := strings.TrimSuffix(
				strings.ReplaceAll(mockld.PollingPathJSClientGet, mockld.PollingPathEnvIDParam, envIDOrMobileKey),
				mockld.PollingPathContextBase64Param, // details of base64-encoded context data are tested separately
			)
			jsReportPath := strings.ReplaceAll(mockld.PollingPathJSClientReport, mockld.PollingPathEnvIDParam, envIDOrMobileKey)
			return h.IfElse(method == flagRequestREPORT,
				m.Equal(jsReportPath),
				m.StringHasPrefix(jsGetPathPrefix))

		default:
			panic("invalid SDK kind")
		}
	}
	pollTests.RequestURLPath(t, requestPathMatcher)

	getPath := h.IfElse(sdkKind == mockld.MobileSDK || sdkKind == mockld.RokuSDK,
		mockld.PollingPathMobileGet,
		strings.ReplaceAll(mockld.PollingPathJSClientGet, mockld.PollingPathEnvIDParam, envIDOrMobileKey))
	pollTests.RequestContextProperties(t, getPath)
}
