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

	t.Run("returns to normal-regime cadence after two consecutive successful polls", func(t *ldtest.T) {
		// Verify RETRY §1.8.1: after the extended regime engages, two consecutive successful polls
		// return the SDK to normal-regime cadence.
		//
		// Flow:
		//   1. SDK polls, gets 401 -> enters extended regime.
		//   2. Poll 2 (~2.5-5 min later) succeeds.
		//   3. Poll 3 (~30 s later) succeeds. Two consecutive successes -> regime resets.
		//   4. Poll 4 returns 401 to induce a fresh fault at NORMAL-regime timing.
		//   5. Poll 5 should arrive within the normal PollInterval (30 s), not the extended
		//      window (~5 min). Under the extended regime, poll 5 would arrive ~5 min later.
		pollSource1 := NewSDKDataSourceWithoutEndpoint(t, dataV1, DataSourceOptionPolling())
		pollSource2 := NewSDKDataSourceWithoutEndpoint(t, dataV1, DataSourceOptionPolling())
		pollSource3 := NewSDKDataSourceWithoutEndpoint(t, dataV1, DataSourceOptionPolling())
		handler := httphelpers.SequentialHandler(
			httphelpers.HandlerWithStatus(401), // 1st: unexpected error -> extended regime
			pollSource1.PollingService(),       // 2nd: success (extended-regime retry)
			pollSource2.PollingService(),       // 3rd: success -> two consecutive successes reset
			httphelpers.HandlerWithStatus(401), // 4th: fault at normal-regime timing
			pollSource3.PollingService(),       // 5th: normal-regime retry (~30 s, not ~5 min)
		)
		pollEndpoint := makePollEndpoint(t, handler)
		t.Defer(pollEndpoint.Close)

		_ = NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{InitCanFail: true}),
			WithPollingConfig(basePollConfig(pollEndpoint)))

		// 1st poll: 401 -> extended regime
		_ = pollEndpoint.RequireConnection(t, initialPollTimeout)

		// 2nd poll: extended-regime retry (~2.5-5 min)
		_ = pollEndpoint.RequireConnection(t, extendedRegimePollTimeout)

		// 3rd poll: arrives at normal PollInterval cadence (30 s server-side minimum).
		normalRegimePollTimeout := 60 * time.Second
		_ = pollEndpoint.RequireConnection(t, normalRegimePollTimeout)

		// 4th poll: fault at normal-regime timing (SDK is now back in normal regime after
		// two consecutive successes, so this poll happens ~30 s after poll 3).
		_ = pollEndpoint.RequireConnection(t, normalRegimePollTimeout)

		// 5th poll: normal-regime retry after the 4th poll's 401. Should arrive within
		// PollInterval (30 s), NOT extended-regime timing (~5 min). If the regime hadn't
		// reset, this timeout would fail and the test surfaces the bug.
		_ = pollEndpoint.RequireConnection(t, normalRegimePollTimeout)
	})
}
