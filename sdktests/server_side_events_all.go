package sdktests

import (
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/servicedef"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
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

	commonTests := CommonEventTests{
		SDKConfigurers: []SDKConfigurer{
			WithConfig(servicedef.SDKConfigParams{
				Credential: sdkKey,
			}),
		},
	}

	commonTests.RequestMethodAndHeaders(t,
		Header("Authorization").Should(m.Equal(sdkKey)))

	commonTests.RequestURLPath(t, m.Equal("/bulk"))

	commonTests.UniquePayloadIDs(t)
}

func doServerSideEventBufferTests(t *ldtest.T) {
	commonTests := CommonEventTests{}

	userFactory := NewUserFactory("doServerSideEventCapacityTests",
		func(b lduser.UserBuilder) { b.Name("my favorite user") })

	commonTests.BufferBehavior(t, userFactory)
}

func doServerSideEventDisableTest(t *ldtest.T) {
	commonTests := CommonEventTests{}

	commonTests.DisablingEvents(t)
}
