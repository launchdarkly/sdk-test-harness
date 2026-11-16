package sdktests

import (
	"net/http"
	"sync/atomic"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

// doPHPPollResponseHandlingTests verifies that the PHP SDK, which requests flag data from the polling
// service during each evaluation, returns the default value when that request gets a bad response
// (instead of throwing an error out of the evaluation), and does not hold on to the failure once the
// service responds normally again.
func doPHPPollResponseHandlingTests(t *ldtest.T) {
	flagKey := "flag"
	value, defaultValue := ldvalue.String("value"), ldvalue.String("default")
	flag := ldbuilders.NewFlagBuilder(flagKey).Version(1).On(false).OffVariation(0).Variations(value).Build()
	data := mockld.NewServerSDKDataBuilder().Flag(flag).Build()
	context := ldcontext.New("user-key")

	defaultIsReturnedThenRecovers := func(t *ldtest.T, badResponse http.Handler) {
		good := NewSDKDataSourceWithoutEndpoint(t, data, DataSourceOptionPolling())
		var broken atomic.Bool
		broken.Store(true)
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if broken.Load() {
				badResponse.ServeHTTP(w, r)
				return
			}
			good.PollingService().ServeHTTP(w, r)
		})
		endpoint := requireContext(t).harness.NewMockEndpoint(handler, t.DebugLogger(),
			harness.MockEndpointDescription("polling service"))
		t.Defer(endpoint.Close)

		client := NewSDKClient(t, WithPollingConfig(servicedef.SDKConfigPollingParams{BaseURI: endpoint.BaseURL()}))

		m.In(t).For("evaluation while the service is failing").Assert(
			basicEvaluateFlag(t, client, flagKey, context, defaultValue), m.Equal(defaultValue))
		// Getting all flags while the service is failing must not fail either.
		_ = client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{Context: o.Some(context)})

		broken.Store(false)
		m.In(t).For("evaluation after the service recovered").Assert(
			basicEvaluateFlag(t, client, flagKey, context, defaultValue), m.Equal(value))
	}

	t.Run("default value is returned after HTTP 503 response", func(t *ldtest.T) {
		defaultIsReturnedThenRecovers(t, httphelpers.HandlerWithStatus(http.StatusServiceUnavailable))
	})

	t.Run("default value is returned after truncated response body", func(t *ldtest.T) {
		defaultIsReturnedThenRecovers(t, mockld.TruncatedBodyHandler("application/json", data["flags"][flagKey]))
	})

	t.Run("default value is returned after connection reset during response body", func(t *ldtest.T) {
		defaultIsReturnedThenRecovers(t, mockld.ResetMidBodyHandler("application/json", data["flags"][flagKey]))
	})

	t.Run("default value is returned after connection closed without a response", func(t *ldtest.T) {
		defaultIsReturnedThenRecovers(t, httphelpers.BrokenConnectionHandler())
	})
}
