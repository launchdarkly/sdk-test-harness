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
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

const briefDelay ldtime.UnixMillisecondTime = 1

func baseStreamConfig(endpoint *harness.MockEndpoint) servicedef.SDKConfigStreamingParams {
	return servicedef.SDKConfigStreamingParams{
		BaseURI:             endpoint.BaseURL(),
		InitialRetryDelayMS: o.Some(briefDelay),
	}
}

func doServerSideStreamRetryTests(t *ldtest.T) {
	recoverableErrors := []int{400, 408, 429, 500, 503}
	unexpectedErrors := []int{401, 403, 405} // really all 4xx errors that aren't 400, 408, or 429

	expectedValueV1 := ldvalue.Int(1)
	expectedValueV2 := ldvalue.Int(2)
	flagKey := "flag"
	flagV1, flagV2 := makeFlagVersionsWithValues(flagKey, 1, 2, expectedValueV1, expectedValueV2)
	dataV1 := mockld.NewServerSDKDataBuilder().Flag(flagV1).Build()
	dataV2 := mockld.NewServerSDKDataBuilder().Flag(flagV2).Build()
	context := ldcontext.New("user-key")

	// Because we're setting InitialRetryDelayMS to a very short delay, we expect reconnections to
	// happen quickly - but, execution speed is always unpredictable, so we'll use a timeout for
	// these that is much longer than we expect we'll need. That won't make the tests run any slower
	// than they otherwise would unless the SDK really is hanging and not reconnecting.
	incomingConnectionTimeout := time.Second * 2

	// When we're asserting "there are no more connections", we should use a timeout that isn't too
	// long because that *will* make successful tests run slow, but long enough that we have a
	// reasonable chance of detecting an inappropriate retry that happened promptly.
	noMoreConnectionsTimeout := time.Millisecond * 100

	makeStreamEndpoint := func(t *ldtest.T, handler http.Handler) *harness.MockEndpoint {
		return requireContext(t).harness.NewMockEndpoint(handler, t.DebugLogger(),
			harness.MockEndpointDescription("streaming service"))
	}

	t.Run("retry after stream is closed", func(t *ldtest.T) {
		stream1 := NewSDKDataSourceWithoutEndpoint(t, dataV1)
		stream2 := NewSDKDataSourceWithoutEndpoint(t, dataV2)
		handler := httphelpers.SequentialHandler(
			stream1.Handler(), // first request gets the first stream data
			stream2.Handler(), // second request gets the second stream data
		)
		streamEndpoint := makeStreamEndpoint(t, handler)
		t.Defer(streamEndpoint.Close)

		client := NewSDKClient(t, WithStreamingConfig(baseStreamConfig(streamEndpoint)))
		result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{Context: o.Some(context)})
		m.In(t).Assert(result, EvalAllFlagsValueForKeyShouldEqual(flagKey, expectedValueV1))

		// Get the request info for the first request
		request1 := streamEndpoint.RequireConnection(t, incomingConnectionTimeout)

		// Now cause the stream to close; this should trigger a reconnect
		request1.Cancel()

		// Expect the second request; it succeeds and gets the second stream data
		_ = streamEndpoint.RequireConnection(t, incomingConnectionTimeout)

		// Check that the client got the new data from the second stream
		pollUntilFlagValueUpdated(t, client, flagKey, context, expectedValueV1, expectedValueV2, ldvalue.Null())
	})

	t.Run("initial retry delay is applied", func(t *ldtest.T) {
		// Since execution time in a test environment is highly unpredictable, we can't really make
		// expectations about seeing specific retry delays. But we can at least verify that if we set
		// the initial delay to a very large value, we should not see a reconnection attempt within a
		// short time.

		stream := NewSDKDataSource(t, dataV1)
		client := NewSDKClient(t,
			WithStreamingConfig(servicedef.SDKConfigStreamingParams{
				InitialRetryDelayMS: o.Some(ldtime.UnixMillisecondTime(10000)),
			}),
			stream,
		)
		result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{Context: o.Some(context)})
		m.In(t).Assert(result, EvalAllFlagsValueForKeyShouldEqual(flagKey, expectedValueV1))

		// Get the request info for the first request
		request1 := stream.Endpoint().RequireConnection(t, incomingConnectionTimeout)

		// Now cause the stream to close; this should trigger a reconnect
		request1.Cancel()

		// We set the initial delay to 10 seconds (which, due to our subtractive jitter behavior,
		// means it should be between 5 and 10 seconds), so we should definitely not see another
		// connection attempt within 100 ms.
		//
		// Note that if the SDK configuration options were just not working, so that it was
		// impossible to change the initial retry delay and it remained at its default value of
		// 1 second (which is really 500-1000ms), then this test would still pass because 100ms
		// is too short a timeout. But in that case, the other tests in this file would fail,
		// since they set a very short retry delay and expect to see connections in much less
		// than 500ms. So, the failure condition we're really checking for here is "the SDK does
		// not do a delay at all, it retries immediately".
		stream.Endpoint().RequireNoMoreConnections(t, noMoreConnectionsTimeout)
	})

	shouldRetryAfterErrorOnInitialConnect := func(t *ldtest.T, errorHandler http.Handler) {
		stream := NewSDKDataSourceWithoutEndpoint(t, dataV1)
		handler := httphelpers.SequentialHandler(
			errorHandler,     // first request gets the error
			errorHandler,     // second request also gets the error
			stream.Handler(), // third request succeeds and gets the stream
		)
		streamEndpoint := makeStreamEndpoint(t, handler)
		t.Defer(streamEndpoint.Close)

		client := NewSDKClient(t, WithStreamingConfig(baseStreamConfig(streamEndpoint)))
		result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{Context: o.Some(context)})
		m.In(t).Assert(result, EvalAllFlagsValueForKeyShouldEqual(flagKey, expectedValueV1))

		for i := 0; i < 3; i++ { // expect three requests
			_ = streamEndpoint.RequireConnection(t, incomingConnectionTimeout)
		}

		streamEndpoint.RequireNoMoreConnections(t, noMoreConnectionsTimeout)
	}

	t.Run("retry after IO error on initial connect", func(t *ldtest.T) {
		shouldRetryAfterErrorOnInitialConnect(t, httphelpers.BrokenConnectionHandler())
	})

	t.Run("retry after recoverable HTTP error on initial connect", func(t *ldtest.T) {
		for _, status := range recoverableErrors {
			t.Run(fmt.Sprintf("error %d", status), func(t *ldtest.T) {
				shouldRetryAfterErrorOnInitialConnect(t, httphelpers.HandlerWithStatus(status))
			})
		}
	})

	shouldRetryAfterErrorOnReconnect := func(t *ldtest.T, errorHandler http.Handler) {
		stream1 := NewSDKDataSourceWithoutEndpoint(t, dataV1)
		stream2 := NewSDKDataSourceWithoutEndpoint(t, dataV2)
		handler := httphelpers.SequentialHandler(
			stream1.Handler(), // first request gets the first stream data
			errorHandler,      // second request gets the error
			errorHandler,      // third request also gets the error
			stream2.Handler(), // fourth request gets the second stream data
		)
		streamEndpoint := makeStreamEndpoint(t, handler)
		t.Defer(streamEndpoint.Close)

		client := NewSDKClient(t, WithStreamingConfig(baseStreamConfig(streamEndpoint)))
		result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{Context: o.Some(context)})
		m.In(t).Assert(result, EvalAllFlagsValueForKeyShouldEqual(flagKey, expectedValueV1))

		// Get the request info for the first request
		request1 := streamEndpoint.RequireConnection(t, incomingConnectionTimeout)

		// Now cause the stream to close; this should trigger a reconnect
		request1.Cancel()

		// Expect the second request; it will receive an error, causing another attempt
		_ = streamEndpoint.RequireConnection(t, incomingConnectionTimeout)

		// Expect the third request; it will also receive an error, causing another attempt
		_ = streamEndpoint.RequireConnection(t, incomingConnectionTimeout)

		// expect the fourth request; this one succeeds and gets the second stream data
		_ = streamEndpoint.RequireConnection(t, incomingConnectionTimeout)

		// check that the client got the new data from the second stream
		pollUntilFlagValueUpdated(t, client, flagKey, context, expectedValueV1, expectedValueV2, ldvalue.Null())
	}

	t.Run("retry after IO error on reconnect", func(t *ldtest.T) {
		shouldRetryAfterErrorOnReconnect(t, httphelpers.BrokenConnectionHandler())
	})

	t.Run("retry after recoverable HTTP error on reconnect", func(t *ldtest.T) {
		for _, status := range recoverableErrors {
			t.Run(fmt.Sprintf("error %d", status), func(t *ldtest.T) {
				shouldRetryAfterErrorOnReconnect(t, httphelpers.HandlerWithStatus(status))
			})
		}
	})

	// extendedRegimeConnectionTimeout is the observation window for a reconnect during
	// extended-regime backoff. The SDK's production default is a 5-minute extended-initial
	// delay with up to 50% subtractive jitter, so the actual wait to the FIRST extended
	// retry is 2.5-5 min. 10 seconds extra gives margin over the 5-min ceiling.
	extendedRegimeConnectionTimeout := 5*time.Minute + 10*time.Second

	// The following retry-conformance tests are guarded by CapabilityRetryConformanceFDv1Streaming
	// being PRESENT AND require -enable-long-running-tests. They assert RETRY-conformant behavior
	// against the SDK's production timing (5-minute extended-initial delay by default), so each
	// scenario takes several minutes of wall clock.
	t.Run("retry after unexpected HTTP error on initial connect", func(t *ldtest.T) {
		t.RequireCapability(servicedef.CapabilityRetryConformanceFDv1Streaming)
		t.LongRunning()
		for _, status := range unexpectedErrors {
			t.Run(fmt.Sprintf("error %d", status), func(t *ldtest.T) {
				stream := NewSDKDataSourceWithoutEndpoint(t, dataV1)
				handler := httphelpers.SequentialHandler(
					httphelpers.HandlerWithStatus(status), // 1st: error
					stream.Handler(),                      // 2nd: success (only reached if SDK retries)
				)
				streamEndpoint := makeStreamEndpoint(t, handler)
				t.Defer(streamEndpoint.Close)

				client := NewSDKClient(t,
					WithConfig(servicedef.SDKConfigParams{InitCanFail: true, StartWaitTimeMS: o.Some(ldtime.UnixMillisecondTime(1))}),
					WithStreamingConfig(baseStreamConfig(streamEndpoint)))

				// 1st connection produces the error
				_ = streamEndpoint.RequireConnection(t, incomingConnectionTimeout)

				// 2nd connection arrives after an extended-regime wait, proving the SDK retried
				// rather than permanently stopping.
				_ = streamEndpoint.RequireConnection(t, extendedRegimeConnectionTimeout)

				// RequireConnection returns as soon as the harness accepts the request; the SDK
				// still needs to read the SSE payload and populate its store. Poll until the
				// flag value reflects the second stream's data.
				pollUntilFlagValueUpdated(t, client, flagKey, context, ldvalue.Null(), expectedValueV1, ldvalue.Null())
			})
		}
	})

	t.Run("retry after unexpected HTTP error on reconnect", func(t *ldtest.T) {
		t.RequireCapability(servicedef.CapabilityRetryConformanceFDv1Streaming)
		t.LongRunning()
		for _, status := range unexpectedErrors {
			t.Run(fmt.Sprintf("error %d", status), func(t *ldtest.T) {
				stream1 := NewSDKDataSourceWithoutEndpoint(t, dataV1)
				stream2 := NewSDKDataSourceWithoutEndpoint(t, dataV2)
				handler := httphelpers.SequentialHandler(
					stream1.Handler(),                     // 1st: first stream data
					httphelpers.HandlerWithStatus(status), // 2nd: error
					stream2.Handler(),                     // 3rd: second stream data (extended-regime retry)
				)
				streamEndpoint := makeStreamEndpoint(t, handler)
				t.Defer(streamEndpoint.Close)

				client := NewSDKClient(t, WithStreamingConfig(baseStreamConfig(streamEndpoint)))

				// Confirm initial connection succeeded, then verify V1 is live
				request1 := streamEndpoint.RequireConnection(t, incomingConnectionTimeout)
				result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{Context: o.Some(context)})
				m.In(t).Assert(result, EvalAllFlagsValueForKeyShouldEqual(flagKey, expectedValueV1))

				// Cause the initial stream to close; triggers reconnect. Next connection is
				// the error, then a successful reconnect at extended-regime timing.
				request1.Cancel()
				_ = streamEndpoint.RequireConnection(t, incomingConnectionTimeout)       // 2nd (error)
				_ = streamEndpoint.RequireConnection(t, extendedRegimeConnectionTimeout) // 3rd (success)

				pollUntilFlagValueUpdated(t, client, flagKey, context, expectedValueV1, expectedValueV2, ldvalue.Null())
			})
		}
	})

	t.Run("enters extended-regime backoff after unexpected HTTP error", func(t *ldtest.T) {
		t.RequireCapability(servicedef.CapabilityRetryConformanceFDv1Streaming)
		t.LongRunning()
		stream := NewSDKDataSourceWithoutEndpoint(t, dataV1)
		handler := httphelpers.SequentialHandler(
			httphelpers.HandlerWithStatus(401), // 1st: unexpected error
			stream.Handler(),                   // 2nd: success (only reached if SDK retries)
		)
		streamEndpoint := makeStreamEndpoint(t, handler)
		t.Defer(streamEndpoint.Close)

		_ = NewSDKClient(t,
			WithConfig(servicedef.SDKConfigParams{InitCanFail: true, StartWaitTimeMS: o.Some(ldtime.UnixMillisecondTime(1))}),
			WithStreamingConfig(baseStreamConfig(streamEndpoint)))

		// 1st connection produces the 401
		_ = streamEndpoint.RequireConnection(t, incomingConnectionTimeout)

		// Assertion A: SDK does NOT reconnect within the normal-regime window (100 ms).
		// If the SDK were retrying at normal-regime timing (1 ms) it would already have reconnected.
		streamEndpoint.RequireNoMoreConnections(t, noMoreConnectionsTimeout)

		// Assertion B: SDK DOES reconnect within the extended-regime window (~5 min + jitter margin).
		// If the SDK had permanently stopped, this would fail.
		_ = streamEndpoint.RequireConnection(t, extendedRegimeConnectionTimeout)
	})

	t.Run("does not permanently stop under sustained unexpected HTTP errors", func(t *ldtest.T) {
		// Endpoint returns 401 to every request. SDK should keep retrying at extended-regime
		// cadence. Under production timing, first extended retry is ~2.5-5 min after the fault.
		t.RequireCapability(servicedef.CapabilityRetryConformanceFDv1Streaming)
		t.LongRunning()
		streamEndpoint := makeStreamEndpoint(t, httphelpers.HandlerWithStatus(401))
		t.Defer(streamEndpoint.Close)

		_ = NewSDKClient(t,
			WithConfig(servicedef.SDKConfigParams{InitCanFail: true, StartWaitTimeMS: o.Some(ldtime.UnixMillisecondTime(1))}),
			WithStreamingConfig(baseStreamConfig(streamEndpoint)))

		// 1st connection: initial 401
		_ = streamEndpoint.RequireConnection(t, incomingConnectionTimeout)

		// 2nd connection: extended-regime retry after ~2.5-5 min. Proves the SDK is still
		// retrying rather than permanently stopping.
		_ = streamEndpoint.RequireConnection(t, extendedRegimeConnectionTimeout)
	})

	t.Run("returns to normal-regime backoff after healthy operation", func(t *ldtest.T) {
		// After the extended regime engages, verify the SDK returns to normal-regime backoff
		// after RETRY §1.8's healthy-operation reset. Per the SDK streaming spec, the SDK sets
		// its activeSince marker when the first SSE event is received on a stream, and on the
		// next reconnection attempt resets the regime if at least 60 seconds have elapsed since
		// that marker.
		//
		// Flow:
		//   1. SDK connects, gets 401 -> enters extended regime.
		//   2. Extended-regime retry (~2.5-5 min later) succeeds; SDK receives the initial
		//      stream event and sets activeSince.
		//   3. Server keeps stream open >= 60 s so the reset will fire on the next reconnect.
		//   4. Server closes the stream to induce a reconnect.
		//   5. SDK should retry at NORMAL-regime timing (briefDelay), not extended (~5 min).
		t.RequireCapability(servicedef.CapabilityRetryConformanceFDv1Streaming)
		t.LongRunning()

		stream1 := NewSDKDataSourceWithoutEndpoint(t, dataV1)
		stream2 := NewSDKDataSourceWithoutEndpoint(t, dataV2)
		handler := httphelpers.SequentialHandler(
			httphelpers.HandlerWithStatus(401), // 1st: unexpected error -> extended regime
			stream1.Handler(),                  // 2nd: successful stream (extended-regime retry)
			stream2.Handler(),                  // 3rd: successful reconnect (normal-regime timing)
		)
		streamEndpoint := makeStreamEndpoint(t, handler)
		t.Defer(streamEndpoint.Close)

		_ = NewSDKClient(t,
			WithConfig(servicedef.SDKConfigParams{InitCanFail: true, StartWaitTimeMS: o.Some(ldtime.UnixMillisecondTime(1))}),
			WithStreamingConfig(baseStreamConfig(streamEndpoint)))

		// 1st connection: 401 -> extended regime engaged
		_ = streamEndpoint.RequireConnection(t, incomingConnectionTimeout)

		// 2nd connection: extended-regime retry succeeds (~2.5-5 min later)
		request2 := streamEndpoint.RequireConnection(t, extendedRegimeConnectionTimeout)

		// Keep the stream open >= 60 s so that when the next reconnect is triggered, at least
		// 60 s will have elapsed since activeSince was set on the initial stream event. Give
		// 75 s to be safe. Silent stream is fine here: the spec sets activeSince on the FIRST
		// SSE event received, not on every event.
		time.Sleep(75 * time.Second)

		// Induce a fresh fault. On this reconnection attempt, the SDK evaluates the reset
		// condition and (since >= 60 s have elapsed since activeSince) returns to the normal
		// regime; next retry uses briefDelay, not extended-regime timing (~5 min).
		request2.Cancel()

		// Assertion: the 3rd connection arrives at NORMAL-regime timing (well under the
		// extended-regime window). We use a 30-second budget -- much less than the
		// ~2.5-min minimum extended-regime wait -- so a still-extended SDK would fail this.
		normalRegimeReconnectTimeout := 30 * time.Second
		_ = streamEndpoint.RequireConnection(t, normalRegimeReconnectTimeout)
	})

	// The following two "do not retry" tests describe legacy behavior for SDKs that have not yet
	// adopted the RETRY specification. Under RETRY, `401` / `403` / other `4xx` responses are
	// classified as `unexpected` errors and trigger an extended-regime backoff rather than a
	// permanent stop. These legacy tests are gated behind the ABSENCE of the
	// `retry-conformance-fdv1-streaming` capability; SDKs that declare that capability run the
	// new "retry after unexpected HTTP error" tests instead. Once every SDK reports the
	// capability, these legacy tests can be removed.
	t.Run("do not retry after unexpected HTTP error on initial connect", func(t *ldtest.T) {
		if t.Capabilities().Has(servicedef.CapabilityRetryConformanceFDv1Streaming) {
			t.SkipWithReason("SDK reports " + servicedef.CapabilityRetryConformanceFDv1Streaming +
				"; legacy permanent-stop behavior does not apply")
			return
		}
		for _, status := range unexpectedErrors {
			t.Run(fmt.Sprintf("error %d", status), func(t *ldtest.T) {
				stream := NewSDKDataSourceWithoutEndpoint(t, dataV1)
				handler := httphelpers.SequentialHandler(
					httphelpers.HandlerWithStatus(status), // first request gets the error
					stream.Handler(),                      // second request would succeed and get the stream, but shouldn't happen
				)
				streamEndpoint := makeStreamEndpoint(t, handler)
				t.Defer(streamEndpoint.Close)

				_ = NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{InitCanFail: true}),
					WithStreamingConfig(baseStreamConfig(streamEndpoint)))

				_ = streamEndpoint.RequireConnection(t, incomingConnectionTimeout)

				streamEndpoint.RequireNoMoreConnections(t, noMoreConnectionsTimeout)
			})
		}
	})

	t.Run("do not retry after unexpected HTTP error on reconnect", func(t *ldtest.T) {
		if t.Capabilities().Has(servicedef.CapabilityRetryConformanceFDv1Streaming) {
			t.SkipWithReason("SDK reports " + servicedef.CapabilityRetryConformanceFDv1Streaming +
				"; legacy permanent-stop behavior does not apply")
			return
		}
		for _, status := range unexpectedErrors {
			t.Run(fmt.Sprintf("error %d", status), func(t *ldtest.T) {
				stream := NewSDKDataSourceWithoutEndpoint(t, dataV1)
				handler := httphelpers.SequentialHandler(
					stream.Handler(),                      // first request gets the stream data
					httphelpers.HandlerWithStatus(status), // second request gets the error
					stream.Handler(),                      // third request would get the stream again, but shouldn't happen
				)
				streamEndpoint := makeStreamEndpoint(t, handler)
				t.Defer(streamEndpoint.Close)

				client := NewSDKClient(t, WithStreamingConfig(baseStreamConfig(streamEndpoint)))
				result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{Context: o.Some(context)})
				m.In(t).Assert(result, EvalAllFlagsValueForKeyShouldEqual(flagKey, expectedValueV1))

				// get the request info for the first request
				request1 := streamEndpoint.RequireConnection(t, incomingConnectionTimeout)

				// now cause the stream to close; this should trigger a reconnect
				request1.Cancel()

				// expect the second request; it will receive an error
				_ = streamEndpoint.RequireConnection(t, incomingConnectionTimeout)

				streamEndpoint.RequireNoMoreConnections(t, noMoreConnectionsTimeout)
			})
		}
	})
}
