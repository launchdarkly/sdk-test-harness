package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

func doServerSideStreamTests(t *ldtest.T) {
	t.Run("requests", doServerSideStreamRequestTests)
	t.Run("retry behavior", doServerSideStreamRetryTests)
	t.Run("validation", doServerSideStreamValidationTests)

	t.Run("fdv2", doServerSideFDv2StreamTests)
}

func doServerSideStreamRequestTests(t *ldtest.T) {
	sdkKey := "my-sdk-key"

	streamTests := NewCommonStreamingTests(t, "doServerSideStreamRequestTests",
		WithConfig(servicedef.SDKConfigParams{
			Credential: sdkKey,
		}))

	if t.Capabilities().Has(servicedef.CapabilityHTTPProxy) {
		streamTests.RequestViaHTTPProxy(t)
	}

	streamTests.RequestMethodAndHeaders(t, sdkKey)

	streamTests.RequestURLPath(t, func(flagRequestMethod) m.Matcher {
		return m.Equal(mockld.StreamingPathServerSide)
	})
}

func doServerSideFDv2StreamTests(t *ldtest.T) {
	NewCommonStreamingTests(t, "doServerSideFDv2StreamTests").FDv2(t)
}
