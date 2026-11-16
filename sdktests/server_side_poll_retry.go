package sdktests

import (
	"fmt"
	"net/http"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
)

// doServerSidePollRetryTests exercises the RETRY specification's non-permanent-stop guarantee
// for the FDv1 server-side polling data source. Server-side polling enforces a PollInterval
// minimum of 30 seconds, and the RETRY spec's extended-regime initial delay is 5 minutes in
// the SDK's production configuration. Tests here run at real production timing and are marked
// long-running.
func doServerSidePollRetryTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityServerSidePolling)
	t.RequireCapability(servicedef.CapabilityRetryConformanceFDv1Polling)
	t.LongRunning()

	unexpectedErrors := []int{401, 403, 405}

	expectedValue := ldvalue.Int(1)
	flagKey := "flag"
	flagV1, _ := makeFlagVersionsWithValues(flagKey, 1, 2, expectedValue, ldvalue.Int(2))
	dataV1 := mockld.NewServerSDKDataBuilder().Flag(flagV1).Build()
	context := ldcontext.New("user-key")

	// extendedRegimePollTimeout is the observation window for the retry poll after an
	// unexpected error. Effective delay is max(extendedInitialDelay, PollInterval); with
	// SDK production defaults that's max(5 min, 30 s) = 5 min.  10 seconds extra gives
	// margin over the 5-min ceiling.
	extendedRegimePollTimeout := 5*time.Minute + 10*time.Second

	// initialPollTimeout bounds the SDK's initial poll (no backoff involved; happens on client
	// construction).
	initialPollTimeout := 15 * time.Second

	makePollEndpoint := func(t *ldtest.T, handler http.Handler) *harness.MockEndpoint {
		return requireContext(t).harness.NewMockEndpoint(handler, t.DebugLogger(),
			harness.MockEndpointDescription("polling service"))
	}

	basePollConfig := func(endpoint *harness.MockEndpoint) servicedef.SDKConfigPollingParams {
		return servicedef.SDKConfigPollingParams{
			BaseURI: endpoint.BaseURL(),
		}
	}

	t.Run("retry after unexpected HTTP error on initial poll", func(t *ldtest.T) {
		for _, status := range unexpectedErrors {
			t.Run(fmt.Sprintf("error %d", status), func(t *ldtest.T) {
				pollSource := NewSDKDataSourceWithoutEndpoint(t, dataV1, DataSourceOptionPolling())
				handler := httphelpers.SequentialHandler(
					httphelpers.HandlerWithStatus(status), // 1st: error
					pollSource.PollingService(),           // 2nd: success (only reached if SDK retries)
				)
				pollEndpoint := makePollEndpoint(t, handler)
				t.Defer(pollEndpoint.Close)

				client := NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{InitCanFail: true}),
					WithPollingConfig(basePollConfig(pollEndpoint)))

				// First poll -> error
				_ = pollEndpoint.RequireConnection(t, initialPollTimeout)

				// Second poll arrives after the extended-regime wait, proving the SDK did not
				// permanently stop after the unexpected error.
				_ = pollEndpoint.RequireConnection(t, extendedRegimePollTimeout)

				// RequireConnection returns as soon as the harness accepts the request; the SDK
				// still needs to read the response body, parse it, and populate its store. Poll
				// until the flag value reflects the second poll's data.
				pollUntilFlagValueUpdated(t, client, flagKey, context, ldvalue.Null(), expectedValue, ldvalue.Null())
			})
		}
	})

	t.Run("resets retry state after two consecutive successful polls", func(t *ldtest.T) {
		// After the extended regime engages, two consecutive successful polls reset the retry
		// state. After any successful poll the SDK polls at its interval, so the cadence alone
		// does not show a reset. We prove the reset with a second 401 after the two successes.
		//
		// An SDK that reset its state waits the first extended-regime wait (2.5 to 5 minutes)
		// after the second 401. An SDK that did not reset waits the second extended-regime wait
		// (5 to 10 minutes). So a conformant SDK always passes, and an SDK that did not reset
		// fails unless its jitter is close to the maximum.
		//
		// Flow:
		//   1. SDK polls, gets 401 -> extended regime.
		//   2. Poll 2 succeeds after the extended-regime wait.
		//   3. Poll 3 succeeds. The two consecutive successes reset the retry state.
		//   4. Poll 4 gets 401 -> extended regime again.
		//   5. Poll 5 must come within the first extended-regime wait.
		pollSource := NewSDKDataSourceWithoutEndpoint(t, dataV1, DataSourceOptionPolling())
		handler := httphelpers.SequentialHandler(
			httphelpers.HandlerWithStatus(401), // 1st: unexpected error -> extended regime
			pollSource.PollingService(),        // 2nd: success 1 (extended-regime wait)
			pollSource.PollingService(),        // 3rd: success 2 -> reset
			httphelpers.HandlerWithStatus(401), // 4th: unexpected error -> extended regime again
			pollSource.PollingService(),        // 5th: success (first extended-regime wait)
		)
		pollEndpoint := makePollEndpoint(t, handler)
		t.Defer(pollEndpoint.Close)

		_ = NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{InitCanFail: true}),
			WithPollingConfig(basePollConfig(pollEndpoint)))

		// 1st poll: 401 -> extended regime
		_ = pollEndpoint.RequireConnection(t, initialPollTimeout)

		// 2nd poll: first success, after the extended-regime wait
		_ = pollEndpoint.RequireConnection(t, extendedRegimePollTimeout)

		// 3rd poll: second success. After a successful poll the SDK polls at its interval.
		normalRegimePollTimeout := 60 * time.Second
		_ = pollEndpoint.RequireConnection(t, normalRegimePollTimeout)

		// 4th poll: 401 at the poll interval
		_ = pollEndpoint.RequireConnection(t, normalRegimePollTimeout)

		// 5th poll: this is the reset proof. It must come within the first extended-regime
		// wait, not the second extended-regime wait of an SDK that did not reset.
		_ = pollEndpoint.RequireConnection(t, extendedRegimePollTimeout)
	})
}
