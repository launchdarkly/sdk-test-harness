package sdktests

import (
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"
)

// Although the PHP SDK is a server-side SDK, it has different analytics event behavior than
// the other server-side SDKs due to its stateless nature. The events it produces use a
// different schema in which there are no summary events, every evaluation causes a feature
// event, and every event contains inline user properties.
//
// In most cases, we have not implemented completely different test methods; we call the
// same server-side test methods, but the underlying implementations in common_tests_events_*
// will use slightly different logic when they detect that we are testing the PHP SDK.

func doPHPEventTests(t *ldtest.T) {
	t.Run("requests", doPHPEventRequestTests)
	t.Run("feature events", doPHPFeatureEventTests)
	t.Run("feature prerequisite events", doServerSideFeaturePrerequisiteEventTests)
	t.Run("experimentation", doServerSideExperimentationEventTests)
	t.Run("identify events", doServerSideIdentifyEventTests)
	t.Run("custom events", doServerSideCustomEventTests)
	t.Run("context properties", doServerSideEventContextTests)
}

func doPHPEventRequestTests(t *ldtest.T) {
	sdkKey := "my-sdk-key"

	eventTests := NewCommonEventTests(t, "doPHPEventRequestTests",
		WithConfig(servicedef.SDKConfigParams{
			Credential: sdkKey,
		}))

	eventTests.RequestMethodAndHeaders(t, sdkKey,
		Header("X-LaunchDarkly-Event-Schema").Should(m.Equal(phpEventSchema)))

	eventTests.RequestURLPath(t, m.Equal("/bulk"))
}
