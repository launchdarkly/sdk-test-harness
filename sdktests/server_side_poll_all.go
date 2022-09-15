package sdktests

import (
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
	"net/http"
)

func doServerSidePollTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityServerSidePolling)

	t.Run("requests", doServerSidePollRequestTests)
}

func makePollEndpoint(t *ldtest.T, handler http.Handler) *harness.MockEndpoint {
	return requireContext(t).harness.NewMockEndpoint(handler, t.DebugLogger(),
		harness.MockEndpointDescription("polling service"))
}

func doServerSidePollRequestTests(t *ldtest.T) {
	sdkKey := "my-sdk-key"
	//recoverableErrors := []int{400, 408, 429, 500, 503}
	//unrecoverableErrors := []int{401, 403, 405}

	pollTests := NewCommonPollingTests(t, "doServerSidePollRequestTests",
		WithConfig(servicedef.SDKConfigParams{
			Credential: sdkKey,
		}))

	pollTests.RequestMethodAndHeaders(t, sdkKey)

	pollTests.RequestURLPath(t, func(flagRequestMethod) m.Matcher {
		return m.Equal(mockld.PollingPathServerSide)
	})

	pollTests.ShouldRetryAfterError(t)

	//t.Run("retry after recoverable HTTP error on reconnect", func(t *ldtest.T) {
	//	for _, status := range recoverableErrors {
	//		t.Run(fmt.Sprintf("error %d", status), func(t *ldtest.T) {
	//			shouldRetryAfterErrorOnInitialConnection(t, httphelpers.HandlerWithStatus(status))
	//		})
	//	}
	//})
}
