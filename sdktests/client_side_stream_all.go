package sdktests

import (
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
		case mockld.ServerSideSDK:
			return m.Equal("/all")

		case mockld.MobileSDK:
			return h.IfElse(method == flagRequestREPORT,
				m.Equal("/meval"),
				m.StringHasPrefix("/meval/")) // details of base64-encoded user data are tested separately

		case mockld.JSClientSDK:
			return h.IfElse(method == flagRequestREPORT,
				m.Equal("/eval/"+envIDOrMobileKey),
				m.StringHasPrefix("/eval/"+envIDOrMobileKey+"/")) // details of base64-encoded user data are tested separately

		default:
			panic("unknown SDK kind")
		}
	}
	streamTests.RequestURLPath(t, requestPathMatcher)

	streamTests.RequestUserProperties(t)
}

func doClientSideStreamUpdateTests(t *ldtest.T) {
	NewCommonStreamingTests(t, "doClientSideStreamUpdateTests").Updates(t)
}
