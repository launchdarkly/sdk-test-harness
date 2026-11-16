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
// the FDv1 client-side polling data source. The tests use a 1-second poll interval, so that the
// normal-regime waits are short and clearly different from the 5-minute extended initial delay.
// The IO error and recoverable HTTP error tests take seconds and run in the normal suite. The
// tests that wait for the extended regime run at real production timing and are marked
// long-running.
func doClientSidePollRetryTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityRetryConformanceFDv1Polling)

	c := NewCommonPollingTests(t, "doClientSidePollRetryTests")

	recoverableErrors := []int{400, 408, 429, 500, 503}
	unexpectedErrors := []int{401, 403, 405} // really all 4xx errors that aren't 400, 408, or 429

	expectedValueV1 := ldvalue.Int(1)
	expectedValueV2 := ldvalue.Int(2)
	flagKey := "flag"
	dataV1 := mockld.NewClientSDKDataBuilder().
		Flag(flagKey, mockld.ClientSDKFlag{Version: 1, Value: expectedValueV1}).
		Build()
	dataV2 := mockld.NewClientSDKDataBuilder().
		Flag(flagKey, mockld.ClientSDKFlag{Version: 2, Value: expectedValueV2}).
		Build()
	context := ldcontext.New("user-key")

	// The test service must apply this interval exactly, also if it is below the SDK's minimum.
	const pollInterval = time.Second

	// A poll after a successful poll, or after a normal failure, comes one interval later. No wait
	// is shorter than the interval. The early margin covers SDKs that time the next poll from the
	// start of the previous poll, because the harness sees that poll a little after it starts.
	// The late margin covers slow scheduling on CI machines.
	intervalWaitMinimum := pollInterval - 250*time.Millisecond
	intervalWaitMaximum := pollInterval + 5*time.Second

	// A poll after an unexpected failure waits the extended initial delay of max(5 minutes, poll
	// interval), less jitter of up to 50%, so 2.5 to 5 minutes. Jitter is optional for polling,
	// so an SDK without jitter waits exactly 5 minutes. Each bound has 10 seconds of margin.
	//
	// These windows assume this attempts rule: the first unexpected failure sets attempts to 1,
	// and each later failure, normal or unexpected, adds 1 to attempts. The RETRY specification
	// leaves this rule to the component specification.
	extendedWaitMinimum := 2*time.Minute + 20*time.Second
	extendedWaitMaximum := 5*time.Minute + 10*time.Second

	// initialPollTimeout bounds the SDK's initial poll, which happens when the client starts.
	initialPollTimeout := 15 * time.Second

	makePollEndpoint := func(t *ldtest.T, handler http.Handler) *harness.MockEndpoint {
		return requireContext(t).harness.NewMockEndpoint(handler, t.DebugLogger(),
			harness.MockEndpointDescription("polling service"))
	}

	// A failed poll does not make a client-side SDK unusable, and under the RETRY specification
	// initialization may not complete until the retry succeeds. So we let initialization fail,
	// and we do not make the test service wait for it.
	//
	// Each client gets a unique context, so that flag data that an SDK cached for an earlier
	// client cannot delay the first poll or satisfy the flag checks.
	makeClient := func(t *ldtest.T, endpoint *harness.MockEndpoint) *SDKClient {
		return NewSDKClient(t, append(
			[]SDKConfigurer{WithConfig(servicedef.SDKConfigParams{
				InitCanFail:     true,
				StartWaitTimeMS: o.Some(ldtime.UnixMillisecondTime(1)),
			})},
			c.baseSDKConfigurationPlus(
				WithClientSideInitialContext(c.contextFactory.NextUniqueContext()),
				WithPollingConfig(servicedef.SDKConfigPollingParams{
					BaseURI:        endpoint.BaseURL(),
					PollIntervalMS: o.Some(ldtime.UnixMillisecondTime(pollInterval / time.Millisecond)),
				}),
			)...)...)
	}

	// requireNextPollBetween fails the test if the next poll comes before minWait or after maxWait.
	requireNextPollBetween := func(t *ldtest.T, endpoint *harness.MockEndpoint, minWait, maxWait time.Duration) {
		endpoint.RequireNoMoreConnections(t, minWait)
		_ = endpoint.RequireConnection(t, maxWait-minWait)
	}

	requireFlagValue := func(t *ldtest.T, client *SDKClient, value ldvalue.Value) {
		h.RequireEventually(t, func() bool {
			return basicEvaluateFlag(t, client, flagKey, context, ldvalue.Null()).Equal(value)
		}, time.Second*5, time.Millisecond*50, "timed out without seeing flag data from the poll")
	}

	shouldRetryAfterErrorOnInitialPoll := func(t *ldtest.T, errorHandler http.Handler) {
		pollSource := NewSDKDataSourceWithoutEndpoint(t, dataV1, DataSourceOptionPolling())
		handler := httphelpers.SequentialHandler(
			errorHandler,                // 1st: error
			pollSource.PollingService(), // 2nd: success (only reached if SDK retries)
		)
		pollEndpoint := makePollEndpoint(t, handler)
		t.Defer(pollEndpoint.Close)

		client := makeClient(t, pollEndpoint)

		_ = pollEndpoint.RequireConnection(t, initialPollTimeout)                         // 1st (error)
		requireNextPollBetween(t, pollEndpoint, intervalWaitMinimum, intervalWaitMaximum) // 2nd (success)
		requireFlagValue(t, client, expectedValueV1)
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

		_ = pollEndpoint.RequireConnection(t, initialPollTimeout)                         // 1st (success)
		requireNextPollBetween(t, pollEndpoint, intervalWaitMinimum, intervalWaitMaximum) // 2nd (error)
		requireNextPollBetween(t, pollEndpoint, intervalWaitMinimum, intervalWaitMaximum) // 3rd (success)

		pollUntilFlagValueUpdated(t, client, flagKey, context, expectedValueV1, expectedValueV2, ldvalue.Null())
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
		t.LongRunning()
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

				// The second poll must come at the extended-regime wait. An SDK that polls again at
				// its interval fails the lower bound, and an SDK that permanently stopped fails the
				// upper bound.
				requireNextPollBetween(t, pollEndpoint, extendedWaitMinimum, extendedWaitMaximum)

				// The SDK must also read the poll data, not only send the request.
				requireFlagValue(t, client, expectedValueV1)
			})
		}
	})

	t.Run("resets retry state after two consecutive successful polls", func(t *ldtest.T) {
		t.LongRunning()
		// After the extended regime engages, two consecutive successful polls reset the retry
		// state. We prove the reset with a second unexpected error after the two successes.
		// After any successful poll the SDK polls at its interval, so the cadence alone does not
		// show a reset.
		//
		// An SDK that reset its state waits the first extended-regime wait (2.5 to 5 minutes)
		// after the second error. An SDK that did not reset waits the second extended-regime wait
		// (5 to 10 minutes). So a conformant SDK always passes, and an SDK that did not reset
		// fails unless its jitter is close to the maximum.
		//
		// This test cannot tell a reset after one success from a reset after two. That would
		// need a 401 directly after one success, and the wait of a conformant SDK after that
		// 401 is random between 5 and 10 minutes, so it cannot be bounded well.
		//
		// Flow:
		//   1. SDK polls, gets 401, and enters the extended regime.
		//   2. Poll 2 succeeds after the extended-regime wait.
		//   3. Poll 3 succeeds at the poll interval. The two successes reset the retry state.
		//   4. Poll 4 gets 401 again, and the SDK enters the extended regime again.
		//   5. Poll 5 must come at the first extended-regime wait, and it serves new data.
		pollSource1 := NewSDKDataSourceWithoutEndpoint(t, dataV1, DataSourceOptionPolling())
		pollSource2 := NewSDKDataSourceWithoutEndpoint(t, dataV2, DataSourceOptionPolling())
		handler := httphelpers.SequentialHandler(
			httphelpers.HandlerWithStatus(401), // 1st: unexpected error -> extended regime
			pollSource1.PollingService(),       // 2nd: success 1 (extended-regime wait)
			pollSource1.PollingService(),       // 3rd: success 2 -> reset
			httphelpers.HandlerWithStatus(401), // 4th: unexpected error -> extended regime again
			pollSource2.PollingService(),       // 5th: success with new data
		)
		pollEndpoint := makePollEndpoint(t, handler)
		t.Defer(pollEndpoint.Close)

		client := makeClient(t, pollEndpoint)

		// 1st poll: 401 -> extended regime
		_ = pollEndpoint.RequireConnection(t, initialPollTimeout)

		// 2nd poll: first success, after the extended-regime wait
		requireNextPollBetween(t, pollEndpoint, extendedWaitMinimum, extendedWaitMaximum)
		requireFlagValue(t, client, expectedValueV1)

		// 3rd poll: second success, at the poll interval
		requireNextPollBetween(t, pollEndpoint, intervalWaitMinimum, intervalWaitMaximum)

		// 4th poll: 401 at the poll interval
		requireNextPollBetween(t, pollEndpoint, intervalWaitMinimum, intervalWaitMaximum)

		// 5th poll: this is the reset proof. It must come at the first extended-regime wait, not
		// the second extended-regime wait of an SDK that did not reset.
		requireNextPollBetween(t, pollEndpoint, extendedWaitMinimum, extendedWaitMaximum)

		pollUntilFlagValueUpdated(t, client, flagKey, context, expectedValueV1, expectedValueV2, ldvalue.Null())
	})
}
