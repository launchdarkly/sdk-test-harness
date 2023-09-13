package sdktests

import (
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

func doClientSideEventTests(t *ldtest.T) {
	t.Run("requests", doClientSideEventRequestTests)
	t.Run("summary events", doClientSideSummaryEventTests)
	t.Run("feature events", doClientSideFeatureEventTests)
	t.Run("debug events", doClientSideDebugEventTests)
	t.Run("experimentation", doClientSideExperimentationEventTests)
	t.Run("identify events", doClientSideIdentifyEventTests)
	t.Run("custom events", doClientSideCustomEventTests)
	t.Run("context properties", doClientSideEventContextTests)
	t.Run("event capacity", doClientSideEventBufferTests)
	t.Run("disabling", doClientSideEventDisableTests)
}

func doClientSideEventRequestTests(t *ldtest.T) {
	sdkKind := requireContext(t).sdkKind
	envIDOrMobileKey := "my-credential"

	eventTests := NewCommonEventTests(t, "doClientSideEventRequestTests",
		WithCredential(envIDOrMobileKey))

	eventTests.RequestMethodAndHeaders(t, envIDOrMobileKey, m.AllOf(
		Header("X-LaunchDarkly-Event-Schema").Should(m.Equal(currentEventSchema)),
		Header("X-LaunchDarkly-Payload-Id").Should(m.Not(m.Equal(""))),
		Header("Content-Type").Should(m.StringContains("application/json")),
	))

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
	NewCommonEventTests(t, "doClientSideIdentifyEventTests").
		IdentifyEvents(t)
}

func doClientSideCustomEventTests(t *ldtest.T) {
	NewCommonEventTests(t, "doClientSideCustomEventTests").
		CustomEvents(t)
}

func doClientSideEventContextTests(t *ldtest.T) {
	NewCommonEventTests(t, "doClientSideEventContextTests").
		EventContexts(t)
}

func doClientSideEventBufferTests(t *ldtest.T) {
	NewCommonEventTests(t, "doClientSideEventBufferTests").
		BufferBehavior(t)
}

func doClientSideEventDisableTests(t *ldtest.T) {
	NewCommonEventTests(t, "doClientSideEventDisableTests").
		DisablingEvents(t)
}
