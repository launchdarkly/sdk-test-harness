package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

func doServerSideEventTests(t *ldtest.T) {
	t.Run("requests", doServerSideEventRequestTests)
	t.Run("summary events", doServerSideSummaryEventTests)
	t.Run("feature events", doServerSideFeatureEventTests)
	t.Run("debug events", doServerSideDebugEventTests)
	t.Run("feature prerequisite events", doServerSideFeaturePrerequisiteEventTests)
	t.Run("experimentation", doServerSideExperimentationEventTests)
	t.Run("identify events", doServerSideIdentifyEventTests)
	t.Run("custom events", doServerSideCustomEventTests)
	t.Run("alias events", doServerSideAliasEventTests)
	t.Run("index events", doServerSideIndexEventTests)
	t.Run("user properties", doServerSideEventUserTests)
	t.Run("event capacity", doServerSideEventBufferTests)
	t.Run("disabling", doServerSideEventDisableTest)
}

func doServerSideEventRequestTests(t *ldtest.T) {
	sdkKey := "my-sdk-key"

	eventTests := NewServerSideEventTests("doServerSideEventRequestTests",
		WithConfig(servicedef.SDKConfigParams{
			Credential: sdkKey,
		}))

	eventTests.RequestMethodAndHeaders(t,
		Header("Authorization").Should(m.Equal(sdkKey)))

	eventTests.RequestURLPath(t, m.Equal("/bulk"))

	eventTests.UniquePayloadIDs(t)
}

func doServerSideIdentifyEventTests(t *ldtest.T) {
	NewServerSideEventTests("doServerSideIdentifyEventTests").
		IdentifyEvents(t)
}

func doServerSideCustomEventTests(t *ldtest.T) {
	NewServerSideEventTests("doServerSideCustomEventTests").
		CustomEvents(t)
}

func doServerSideAliasEventTests(t *ldtest.T) {
	NewServerSideEventTests("doServerSideAliasEventTests").
		AliasEvents(t)
}

func doServerSideEventUserTests(t *ldtest.T) {
	NewServerSideEventTests("doServerSideEventUserTests").
		EventUsers(t)
}

func doServerSideEventBufferTests(t *ldtest.T) {
	NewServerSideEventTests("doServerSideEventCapacityTests").
		BufferBehavior(t)
}

func doServerSideEventDisableTest(t *ldtest.T) {
	NewServerSideEventTests("doServerSideEventDisableTest").
		DisablingEvents(t)
}