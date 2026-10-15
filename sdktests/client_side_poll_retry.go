package sdktests

import (
	"fmt"
	"net/http"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
)

// doClientSidePollRetryTests exercises the RETRY specification's non-permanent-stop guarantee for
// the FDv1 client-side polling data source. Client-side SDKs commonly enforce a 5-minute minimum
// poll interval, which is also the production extended initial delay. Tests here run at real
// production timing and are marked long-running.
func doClientSidePollRetryTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityRetryConformanceFDv1Polling)
	t.LongRunning()

	c := NewCommonPollingTests(t, "doClientSidePollRetryTests")

	recoverableErrors := []int{400, 408, 429, 500, 503}
	unexpectedErrors := []int{401, 403, 405} // really all 4xx errors that aren't 400, 408, or 429

	expectedValue := ldvalue.Int(1)
	flagKey := "flag"
	dataV1 := mockld.NewClientSDKDataBuilder().
		Flag(flagKey, mockld.ClientSDKFlag{Version: 1, Value: expectedValue}).
		Build()
	expectedValueV2 := ldvalue.Int(2)
	dataV2 := mockld.NewClientSDKDataBuilder().
		Flag(flagKey, mockld.ClientSDKFlag{Version: 2, Value: expectedValueV2}).
		Build()
	context := ldcontext.New("user-key")

	// pollInterval is the shortest interval that client-side SDKs commonly allow. We set it
	// explicitly so that the expected timing does not depend on the SDK's default.
	const pollInterval = 5 * time.Minute

	// pollTimingMargin allows for timing differences between the SDK and the harness. The SDK
	// starts its timer only after it reads the previous response.
	pollTimingMargin := 10 * time.Second

	// A retry after an unexpected error waits max(extended initial delay, poll interval), less
	// jitter, but never less than the poll interval. With a 5-minute interval that is 5 minutes.
	extendedRegimeMaximumWait := max(5*time.Minute, pollInterval)

	// initialPollTimeout bounds the SDK's initial poll, which happens when the client starts.
	initialPollTimeout := 15 * time.Second

	makePollEndpoint := func(t *ldtest.T, handler http.Handler) *harness.MockEndpoint {
		return requireContext(t).harness.NewMockEndpoint(handler, t.DebugLogger(),
			harness.MockEndpointDescription("polling service"))
	}

	// A failed poll does not make a client-side SDK unusable, and under the RETRY specification
	// initialization may not complete until the retry succeeds. So we let initialization fail,
	// and we do not make the test service wait for it.
	makeClient := func(t *ldtest.T, endpoint *harness.MockEndpoint) *SDKClient {
		return NewSDKClient(t, append(
			[]SDKConfigurer{WithConfig(servicedef.SDKConfigParams{
				InitCanFail:     true,
				StartWaitTimeMS: o.Some(ldtime.UnixMillisecondTime(1)),
			})},
			c.baseSDKConfigurationPlus(
				WithPollingConfig(servicedef.SDKConfigPollingParams{
					BaseURI:        endpoint.BaseURL(),
					PollIntervalMS: o.Some(ldtime.UnixMillisecondTime(pollInterval / time.Millisecond)),
				}),
			)...)...)
	}

	// requireNextPollBetween fails the test if the next poll comes before minWait or after maxWait.
	requireNextPollBetween := func(t *ldtest.T, endpoint *harness.MockEndpoint, minWait, maxWait time.Duration) {
		endpoint.RequireNoMoreConnections(t, minWait-pollTimingMargin)
		_ = endpoint.RequireConnection(t, maxWait-minWait+2*pollTimingMargin)
	}

	// A retry after a normal failure waits the poll interval. These tests only check that the
	// SDK keeps polling and recovers, so they allow up to the interval plus the margin.
	normalRegimePollTimeout := pollInterval + pollTimingMargin

	shouldRetryAfterErrorOnInitialPoll := func(t *ldtest.T, errorHandler http.Handler) {
		pollSource := NewSDKDataSourceWithoutEndpoint(t, dataV1, DataSourceOptionPolling())
		handler := httphelpers.SequentialHandler(
			errorHandler,                // 1st: error
			pollSource.PollingService(), // 2nd: success (only reached if SDK retries)
		)
		pollEndpoint := makePollEndpoint(t, handler)
		t.Defer(pollEndpoint.Close)

		client := makeClient(t, pollEndpoint)

		_ = pollEndpoint.RequireConnection(t, initialPollTimeout)      // 1st (error)
		_ = pollEndpoint.RequireConnection(t, normalRegimePollTimeout) // 2nd (success)

		h.RequireEventually(t, func() bool {
			return basicEvaluateFlag(t, client, flagKey, context, ldvalue.Null()).Equal(expectedValue)
		}, time.Second*5, time.Millisecond*50, "timed out without seeing flag data from the poll")
	}

	t.Run("retry after IO error on initial poll", func(t *ldtest.T) {
		shouldRetryAfterErrorOnInitialPoll(t, httphelpers.BrokenConnectionHandler())
	})

	t.Run("retry after recoverable HTTP error on initial poll", func(t *ldtest.T) {
		for _, status := range recoverableErrors {
			t.Run(fmt.Sprintf("error %d", status), func(t *ldtest.T) {
				shouldRetryAfterErrorOnInitialPoll(t, httphelpers.HandlerWithStatus(status))
			})
		}
	})

	shouldRetryAfterErrorOnLaterPoll := func(t *ldtest.T, errorHandler http.Handler) {
		pollSource1 := NewSDKDataSourceWithoutEndpoint(t, dataV1, DataSourceOptionPolling())
		pollSource2 := NewSDKDataSourceWithoutEndpoint(t, dataV2, DataSourceOptionPolling())
		handler := httphelpers.SequentialHandler(
			pollSource1.PollingService(), // 1st: first poll data
			errorHandler,                 // 2nd: error
			pollSource2.PollingService(), // 3rd: second poll data
		)
		pollEndpoint := makePollEndpoint(t, handler)
		t.Defer(pollEndpoint.Close)

		client := makeClient(t, pollEndpoint)

		_ = pollEndpoint.RequireConnection(t, initialPollTimeout) // 1st (success)
		h.RequireEventually(t, func() bool {
			return basicEvaluateFlag(t, client, flagKey, context, ldvalue.Null()).Equal(expectedValue)
		}, time.Second*5, time.Millisecond*50, "timed out without seeing flag data from the first poll")

		_ = pollEndpoint.RequireConnection(t, normalRegimePollTimeout) // 2nd (error)
		_ = pollEndpoint.RequireConnection(t, normalRegimePollTimeout) // 3rd (success)

		pollUntilFlagValueUpdated(t, client, flagKey, context, expectedValue, expectedValueV2, ldvalue.Null())
	}

	t.Run("retry after IO error on later poll", func(t *ldtest.T) {
		shouldRetryAfterErrorOnLaterPoll(t, httphelpers.BrokenConnectionHandler())
	})

	t.Run("retry after recoverable HTTP error on later poll", func(t *ldtest.T) {
		for _, status := range recoverableErrors {
			t.Run(fmt.Sprintf("error %d", status), func(t *ldtest.T) {
				shouldRetryAfterErrorOnLaterPoll(t, httphelpers.HandlerWithStatus(status))
			})
		}
	})

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

				client := makeClient(t, pollEndpoint)

				// First poll -> error
				_ = pollEndpoint.RequireConnection(t, initialPollTimeout)

				// The second poll must not come before the poll interval, and must come by the end
				// of the extended-regime wait. This proves that the SDK did not permanently stop.
				requireNextPollBetween(t, pollEndpoint, pollInterval, extendedRegimeMaximumWait)

				// The SDK must also read the poll data, not only send the request.
				h.RequireEventually(t, func() bool {
					return basicEvaluateFlag(t, client, flagKey, context, ldvalue.Null()).Equal(expectedValue)
				}, time.Second*5, time.Millisecond*50, "timed out without seeing flag data from the poll")
			})
		}
	})

	t.Run("returns to normal-regime cadence after two consecutive successful polls", func(t *ldtest.T) {
		// After the extended regime engages, two consecutive successful polls reset the retry
		// state. We prove the reset with a second unexpected error after the two successes.
		//
		// With a 5-minute poll interval, the first extended-regime wait is exactly 5 minutes,
		// because jitter cannot take the wait below the interval. An SDK that reset its state
		// waits 5 minutes after the second error. An SDK that did not reset doubles the base
		// delay to 10 minutes, and with jitter waits 5 to 10 minutes. So a conformant SDK always
		// passes, and an SDK that did not reset fails unless its jitter is close to the maximum.
		//
		// Flow:
		//   1. SDK polls, gets 401, and enters the extended regime.
		//   2. Poll 2 succeeds after the extended-regime wait.
		//   3. Poll 3 succeeds at the poll interval. The two successes reset the retry state.
		//   4. Poll 4 gets 401 again, and the SDK enters the extended regime again.
		//   5. Poll 5 must come at the first extended-regime wait, and it serves new data.
		pollSource1 := NewSDKDataSourceWithoutEndpoint(t, dataV1, DataSourceOptionPolling())
		pollSource2 := NewSDKDataSourceWithoutEndpoint(t, dataV1, DataSourceOptionPolling())
		pollSource3 := NewSDKDataSourceWithoutEndpoint(t, dataV2, DataSourceOptionPolling())
		handler := httphelpers.SequentialHandler(
			httphelpers.HandlerWithStatus(401), // 1st: unexpected error -> extended regime
			pollSource1.PollingService(),       // 2nd: success 1 (extended-regime wait)
			pollSource2.PollingService(),       // 3rd: success 2 -> reset
			httphelpers.HandlerWithStatus(401), // 4th: unexpected error -> extended regime again
			pollSource3.PollingService(),       // 5th: success with new data
		)
		pollEndpoint := makePollEndpoint(t, handler)
		t.Defer(pollEndpoint.Close)

		client := makeClient(t, pollEndpoint)

		// 1st poll: 401 -> extended regime
		_ = pollEndpoint.RequireConnection(t, initialPollTimeout)

		// 2nd poll: first success, after the extended-regime wait
		requireNextPollBetween(t, pollEndpoint, pollInterval, extendedRegimeMaximumWait)

		// 3rd poll: second success. After a successful poll the SDK polls at its interval.
		requireNextPollBetween(t, pollEndpoint, pollInterval, pollInterval)

		// 4th poll: 401 at the poll interval
		requireNextPollBetween(t, pollEndpoint, pollInterval, pollInterval)

		// 5th poll: this is the reset proof. It must come at the first extended-regime wait, not
		// the doubled wait of an SDK that did not reset.
		requireNextPollBetween(t, pollEndpoint, pollInterval, extendedRegimeMaximumWait)

		pollUntilFlagValueUpdated(t, client, flagKey, context, expectedValue, expectedValueV2, ldvalue.Null())
	})
}
