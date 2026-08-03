package sdktests

import (
	"fmt"
	"net/http"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

// doServerSidePollRetryTests exercises the RETRY specification's non-permanent-stop guarantee
// for the FDv1 server-side polling data source. Server-side polling enforces a PollInterval
// minimum of 30 seconds, so tests here inherently take tens of seconds per iteration and are
// marked LongRunning.
//
// This first pass covers the core guarantee (no permanent stop after an unexpected HTTP error);
// additional polling scenarios (extended-regime shape, reset after 2 consecutive successful
// polls) are deferred to follow-up work.
func doServerSidePollRetryTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityServerSidePolling)
	t.RequireCapability(servicedef.CapabilityRetryConformanceFDv1Polling)
	t.LongRunning() // polling waits are >= 30 seconds per iteration

	unexpectedErrors := []int{401, 403, 405}

	expectedValue := ldvalue.Int(1)
	flagKey := "flag"
	flagV1, _ := makeFlagVersionsWithValues(flagKey, 1, 2, expectedValue, ldvalue.Int(2))
	dataV1 := mockld.NewServerSDKDataBuilder().Flag(flagV1).Build()
	context := ldcontext.New("user-key")

	// pollingRecoveryTimeout is a generous ceiling for observing the second poll request after
	// an unexpected error. It must exceed the SDK's extended-regime initial delay for polling,
	// which is max(ExtendedInitialDelayMS, PollInterval). With PollInterval at the server-side
	// minimum of 30 s, the effective delay is 30 s; the SDK's actual retry has jitter and
	// platform variance on top. 90 s covers this with generous headroom.
	pollingRecoveryTimeout := 90 * time.Second

	// initialPollTimeout bounds the SDK's initial poll (no backoff involved; happens on client
	// construction).
	initialPollTimeout := 15 * time.Second

	makePollEndpoint := func(t *ldtest.T, handler http.Handler) *harness.MockEndpoint {
		return requireContext(t).harness.NewMockEndpoint(handler, t.DebugLogger(),
			harness.MockEndpointDescription("polling service"))
	}

	basePollConfig := func(endpoint *harness.MockEndpoint) servicedef.SDKConfigPollingParams {
		return servicedef.SDKConfigPollingParams{
			BaseURI:                endpoint.BaseURL(),
			ExtendedInitialDelayMS: o.Some(compressedExtendedInitialDelay),
		}
	}

	t.Run("retry after unexpected HTTP error on initial poll", func(t *ldtest.T) {
		for _, status := range unexpectedErrors {
			t.Run(fmt.Sprintf("error %d", status), func(t *ldtest.T) {
				dataSystem := NewSDKDataSystemWithoutEndpoints(t, dataV1, DataSystemOptionPolling())
				handler := httphelpers.SequentialHandler(
					httphelpers.HandlerWithStatus(status), // 1st: error
					dataSystem.Synchronizers[0].polling,   // 2nd: success (only reached if SDK retries)
				)
				pollEndpoint := makePollEndpoint(t, handler)
				t.Defer(pollEndpoint.Close)

				client := NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{InitCanFail: true}),
					WithPollingSynchronizer(basePollConfig(pollEndpoint)))

				// First poll -> error
				_ = pollEndpoint.RequireConnection(t, initialPollTimeout)

				// Second poll should arrive within the extended-regime window, proving the SDK
				// did not permanently stop after the unexpected error.
				_ = pollEndpoint.RequireConnection(t, pollingRecoveryTimeout)

				// SDK reaches ready and evaluates against the second poll's data.
				result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{Context: o.Some(context)})
				m.In(t).Assert(result, EvalAllFlagsValueForKeyShouldEqual(flagKey, expectedValue))
			})
		}
	})
}
