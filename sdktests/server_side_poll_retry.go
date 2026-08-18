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
		// We prove the reset by observing a post-reset poll arriving at normal PollInterval
		// cadence (~30 s) rather than extended-regime cadence (~5 min). We deliberately do NOT
		// inject another 401 after the reset: a 401 would immediately re-enter the extended
		// regime under RETRY, so the following poll would arrive ~5 min later — that would be
		// spec-conformant behavior, not a bug.
		//
		// Flow:
		//   1. SDK polls, gets 401 -> extended regime.
		//   2. Poll 2 succeeds; arrives at extended-regime cadence (~2.5-5 min).
		//   3. Poll 3 succeeds; timing is interpretation-dependent, so we allow up to the
		//      extended window. The two consecutive successes complete §1.8.1's reset.
		//   4. Poll 4 succeeds; MUST arrive at normal PollInterval cadence (~30 s). If the
		//      regime hadn't reset, this poll would take ~5 min and the assertion fails.
		pollSource1 := NewSDKDataSourceWithoutEndpoint(t, dataV1, DataSourceOptionPolling())
		pollSource2 := NewSDKDataSourceWithoutEndpoint(t, dataV1, DataSourceOptionPolling())
		pollSource3 := NewSDKDataSourceWithoutEndpoint(t, dataV1, DataSourceOptionPolling())
		handler := httphelpers.SequentialHandler(
			httphelpers.HandlerWithStatus(401), // 1st: unexpected error -> extended regime
			pollSource1.PollingService(),       // 2nd: success 1 (extended-regime cadence)
			pollSource2.PollingService(),       // 3rd: success 2 -> §1.8.1 reset
			pollSource3.PollingService(),       // 4th: success at normal-regime cadence (proof)
		)
		pollEndpoint := makePollEndpoint(t, handler)
		t.Defer(pollEndpoint.Close)

		_ = NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{InitCanFail: true}),
			WithPollingConfig(basePollConfig(pollEndpoint)))

		// 1st poll: 401 -> extended regime
		_ = pollEndpoint.RequireConnection(t, initialPollTimeout)

		// 2nd poll: first success, arrives at extended-regime cadence
		_ = pollEndpoint.RequireConnection(t, extendedRegimePollTimeout)

		// 3rd poll: second success. Whether this arrives at extended or normal cadence
		// depends on when the SDK internally applies the reset (after 1st or 2nd success),
		// so we allow up to the extended window either way. The reset is complete by the
		// end of this poll per §1.8.1.
		_ = pollEndpoint.RequireConnection(t, extendedRegimePollTimeout)

		// 4th poll: MUST be at normal PollInterval cadence. This is the reset proof: if the
		// regime hadn't reset, this poll would take ~5 min instead of ~30 s.
		normalRegimePollTimeout := 60 * time.Second
		_ = pollEndpoint.RequireConnection(t, normalRegimePollTimeout)
	})
}
