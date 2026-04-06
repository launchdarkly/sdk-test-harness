package sdktests

import (
	"strings"

	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

func doClientSideStreamTests(t *ldtest.T) {
	t.Run("requests", doClientSideStreamRequestTest)
	t.Run("fdv2", doClientSideFDv2StreamTests)
}

func doClientSideStreamRequestTest(t *ldtest.T) {
	sdkKind := requireContext(t).sdkKind
	envIDOrMobileKey := "my-credential"

	streamTests := NewCommonStreamingTests(t, "doClientSideStreamRequestTest",
		WithCredential(envIDOrMobileKey))

	if t.Capabilities().Has(servicedef.CapabilityHTTPProxy) {
		streamTests.RequestViaHTTPProxy(t)
	}

	streamTests.RequestMethodAndHeaders(t, envIDOrMobileKey)

	requestPathMatcher := func(method flagRequestMethod) m.Matcher {
		switch sdkKind {
		case mockld.MobileSDK, mockld.JSClientSDK:
			getPathPrefix := strings.TrimSuffix(
				mockld.StreamingPathFDv2ClientGet,
				mockld.StreamingPathContextBase64Param, // details of base64-encoded context data are tested separately
			)
			return h.IfElse(method == flagRequestREPORT,
				m.Equal(mockld.StreamingPathFDv2ClientPost),
				m.StringHasPrefix(getPathPrefix))
		case mockld.RokuSDK:
			panic("invalid SDK kind")
		default:
			panic("invalid SDK kind")
		}
	}
	streamTests.RequestURLPath(t, requestPathMatcher)

	getPath := h.IfElse(sdkKind == mockld.RokuSDK,
		mockld.StreamingPathMobileGet,
		mockld.StreamingPathFDv2ClientGet)
	streamTests.RequestContextProperties(t, getPath)
}

func doClientSideFDv2StreamTests(t *ldtest.T) {
	NewCommonStreamingTests(t, "doClientSideFDv2StreamTests").FDv2(t)
}
