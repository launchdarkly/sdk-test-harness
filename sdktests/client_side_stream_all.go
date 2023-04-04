package sdktests

import (
	"strings"

	h "github.com/launchdarkly/sdk-test-harness/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

func doClientSideStreamTests(t *ldtest.T) {
	t.Run("requests", doClientSideStreamRequestTest)
	t.Run("updates", doClientSideStreamUpdateTests)
}

func doClientSideStreamRequestTest(t *ldtest.T) {
	sdkKind := requireContext(t).sdkKind
	envIDOrMobileKey := "my-credential"

	streamTests := NewCommonStreamingTests(t, "doClientSideStreamRequestTest",
		WithCredential(envIDOrMobileKey))

	streamTests.RequestMethodAndHeaders(t, envIDOrMobileKey)

	requestPathMatcher := func(method flagRequestMethod) m.Matcher {
		switch sdkKind {
		case mockld.RokuSDK:
			panic("invalid SDK kind")
		case mockld.MobileSDK:
			mobileGetPathPrefix := strings.TrimSuffix(mockld.StreamingPathMobileGet, mockld.StreamingPathUserBase64Param)
			return h.IfElse(method == flagRequestREPORT,
				m.Equal("/meval"),
				m.StringHasPrefix(mobileGetPathPrefix))
			// details of base64-encoded user data are tested separately

		case mockld.JSClientSDK:
			jsGetPathPrefix := strings.TrimSuffix(
				strings.ReplaceAll(mockld.StreamingPathJSClientGet, mockld.StreamingPathEnvIDParam, envIDOrMobileKey),
				mockld.StreamingPathUserBase64Param, // details of base64-encoded user data are tested separately
			)
			jsReportPath := strings.ReplaceAll(mockld.StreamingPathJSClientReport,
				mockld.StreamingPathEnvIDParam, envIDOrMobileKey)
			return h.IfElse(method == flagRequestREPORT,
				m.Equal(jsReportPath),
				m.StringHasPrefix(jsGetPathPrefix))

		default:
			panic("invalid SDK kind")
		}
	}
	streamTests.RequestURLPath(t, requestPathMatcher)

	getPath := h.IfElse(sdkKind == mockld.MobileSDK || sdkKind == mockld.RokuSDK,
		mockld.StreamingPathMobileGet,
		strings.ReplaceAll(mockld.StreamingPathJSClientGet, mockld.PollingPathEnvIDParam, envIDOrMobileKey))
	streamTests.RequestUserProperties(t, getPath)
}

func doClientSideStreamUpdateTests(t *ldtest.T) {
	NewCommonStreamingTests(t, "doClientSideStreamUpdateTests").Updates(t)
}
