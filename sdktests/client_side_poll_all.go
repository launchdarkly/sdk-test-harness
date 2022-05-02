package sdktests

import (
	"strings"

	h "github.com/launchdarkly/sdk-test-harness/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"

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
		case mockld.MobileSDK:
			mobileGetPathPrefix := strings.TrimSuffix(mockld.PollingPathMobileGet, mockld.PollingPathUserBase64Param)
			return h.IfElse(method == flagRequestREPORT,
				m.Equal(mockld.PollingPathMobileReport),
				m.StringHasPrefix(mobileGetPathPrefix))
			// details of base64-encoded user data are tested separately

		case mockld.JSClientSDK:
			jsGetPathPrefix := strings.TrimSuffix(
				strings.ReplaceAll(mockld.PollingPathJSClientGet, mockld.PollingPathEnvIDParam, envIDOrMobileKey),
				mockld.PollingPathUserBase64Param, // details of base64-encoded user data are tested separately
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

	getPath := h.IfElse(sdkKind == mockld.MobileSDK,
		mockld.PollingPathMobileGet,
		strings.ReplaceAll(mockld.PollingPathJSClientGet, mockld.PollingPathEnvIDParam, envIDOrMobileKey))
	pollTests.RequestUserProperties(t, getPath)
}
