package sdktests

import (
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/servicedef"
)

func doServerSideStreamTests(t *ldtest.T) {
	t.Run("requests", doServerSideStreamRequestTests)
	t.Run("updates", doServerSideStreamUpdateTests)
	t.Run("retry behavior", doServerSideStreamRetryTests)
	t.Run("validation", doServerSideStreamValidationTests)
}

func doServerSideStreamRequestTests(t *ldtest.T) {
	sdkKey := "my-sdk-key"

	streamTests := NewCommonStreamingTests(t, "doServerSideStreamRequestTests",
		WithConfig(servicedef.SDKConfigParams{
			Credential: sdkKey,
		}))

	streamTests.RequestMethodAndHeaders(t,
		Header("Authorization").Should(m.Equal(sdkKey)))

	streamTests.RequestURLPath(t, func(flagRequestMethod) m.Matcher {
		return m.Equal("/all")
	})
}

func doServerSideStreamUpdateTests(t *ldtest.T) {
	NewCommonStreamingTests(t, "doServerSideStreamUpdateTests").Updates(t)
}
