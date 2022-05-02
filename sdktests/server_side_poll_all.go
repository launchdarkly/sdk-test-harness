package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

func doServerSidePollTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityServerSidePolling)

	t.Run("requests", doServerSidePollRequestTests)
}

func doServerSidePollRequestTests(t *ldtest.T) {
	sdkKey := "my-sdk-key"

	pollTests := NewCommonPollingTests(t, "doServerSidePollRequestTests",
		WithConfig(servicedef.SDKConfigParams{
			Credential: sdkKey,
		}))

	pollTests.RequestMethodAndHeaders(t, sdkKey)

	pollTests.RequestURLPath(t, func(flagRequestMethod) m.Matcher {
		return m.Equal(mockld.PollingPathServerSide)
	})
}
