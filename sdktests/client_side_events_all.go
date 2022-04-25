package sdktests

import (
	h "github.com/launchdarkly/sdk-test-harness/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

func doClientSideEventTests(t *ldtest.T) {
	t.Run("requests", doClientSideEventRequestTests)
	t.Run("identify events", doClientSideIdentifyEventTests)
	t.Run("custom events", doClientSideCustomEventTests)
	t.Run("alias events", doClientSideAliasEventTests)
	t.Run("user properties", doClientSideEventUserTests)
	t.Run("event capacity", doClientSideEventBufferTests)
	t.Run("disabling", doClientSideEventDisableTests)
}

func doClientSideEventRequestTests(t *ldtest.T) {
	sdkKind := requireContext(t).sdkKind
	envIDOrMobileKey := "my-credential"

	eventTests := NewClientSideEventTests("doClientSideEventRequestTests.MethodAndHeaders",
		WithCredential(envIDOrMobileKey))

	authHeaderMatcher := Header("Authorization").Should(m.Equal(
		h.IfElse(sdkKind == mockld.MobileSDK, envIDOrMobileKey, "")))
	eventTests.RequestMethodAndHeaders(t, authHeaderMatcher)

	requestPathMatcher := h.IfElse(
		sdkKind == mockld.JSClientSDK,
		m.Equal("/events/bulk/"+envIDOrMobileKey),
		m.AnyOf(
			// for mobile, there are several supported paths
			m.Equal("/mobile"),
			m.Equal("/mobile/events"),
			m.Equal("/mobile/events/bulk"),
		),
	)
	eventTests.RequestURLPath(t, requestPathMatcher)

	eventTests.UniquePayloadIDs(t)
}

func doClientSideIdentifyEventTests(t *ldtest.T) {
	NewClientSideEventTests("doClientSideIdentifyEventTests").
		IdentifyEvents(t)
}

func doClientSideCustomEventTests(t *ldtest.T) {
	NewClientSideEventTests("doClientSideCustomEventTests").
		CustomEvents(t)
}

func doClientSideAliasEventTests(t *ldtest.T) {
	NewClientSideEventTests("doClientSideAliasEventTests").
		AliasEvents(t)
}

func doClientSideEventUserTests(t *ldtest.T) {
	NewClientSideEventTests("doClientSideEventUserTests").
		EventUsers(t)
}

func doClientSideEventBufferTests(t *ldtest.T) {
	NewClientSideEventTests("doClientSideEventBufferTests").
		BufferBehavior(t)
}

func doClientSideEventDisableTests(t *ldtest.T) {
	NewClientSideEventTests("doClientSideEventDisableTests").
		DisablingEvents(t)
}