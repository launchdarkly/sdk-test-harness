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

// compressedExtendedInitialDelay is the value used for ExtendedInitialDelayMS in
// retry-conformance tests. Compressed from the SDK's production default of 5 minutes
// so tests can observe extended-regime engagement in a few seconds. Chosen at 500 ms
// to be well above the paired-bound "no more connections" window (100 ms) and low enough
// to keep tests fast.
const compressedExtendedInitialDelay ldtime.UnixMillisecondTime = 500

// retryConformanceStreamConfig returns a streaming config that compresses both the normal-regime
// initial delay and the extended-regime initial delay to observable values. Used by tests guarded
// by CapabilityRetryConformanceFDv1Streaming.
func retryConformanceStreamConfig(endpoint *harness.MockEndpoint) servicedef.SDKConfigStreamingParams {
	return servicedef.SDKConfigStreamingParams{
		BaseURI:                endpoint.BaseURL(),
		InitialRetryDelayMS:    o.Some(briefDelay),
		ExtendedInitialDelayMS: o.Some(compressedExtendedInitialDelay),
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
		dataSystem1 := NewSDKDataSystemWithoutEndpoints(t, dataV1)
		dataSystem2 := NewSDKDataSystemWithoutEndpoints(t, dataV2)

		handler := httphelpers.SequentialHandler(
			dataSystem1.Synchronizers[0].streaming,
			dataSystem2.Synchronizers[0].streaming,
		)
		streamEndpoint := makeStreamEndpoint(t, handler)
		t.Defer(streamEndpoint.Close)

		client := NewSDKClient(t, WithStreamingSynchronizer(baseStreamConfig(streamEndpoint)))
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

		dataSystem := NewSDKDataSystem(t, dataV1)
		client := NewSDKClient(t,
			WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
				InitialRetryDelayMS: o.Some(ldtime.UnixMillisecondTime(10000)),
			}),
			dataSystem,
		)
		result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{Context: o.Some(context)})
		m.In(t).Assert(result, EvalAllFlagsValueForKeyShouldEqual(flagKey, expectedValueV1))

		// Get the request info for the first request
		request1 := dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, incomingConnectionTimeout)

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
		dataSystem.Synchronizers[0].Endpoint().RequireNoMoreConnections(t, noMoreConnectionsTimeout)
	})

	shouldRetryAfterErrorOnInitialConnect := func(t *ldtest.T, errorHandler http.Handler) {
		dataSystem := NewSDKDataSystemWithoutEndpoints(t, dataV1)
		handler := httphelpers.SequentialHandler(
			errorHandler,                          // first request gets the error
			errorHandler,                          // second request also gets the error
			dataSystem.Synchronizers[0].streaming, // third request succeeds and gets the stream
		)
		streamEndpoint := makeStreamEndpoint(t, handler)
		t.Defer(streamEndpoint.Close)

		client := NewSDKClient(t, WithStreamingSynchronizer(baseStreamConfig(streamEndpoint)))
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
		dataSystem1 := NewSDKDataSystemWithoutEndpoints(t, dataV1)
		dataSystem2 := NewSDKDataSystemWithoutEndpoints(t, dataV2)

		handler := httphelpers.SequentialHandler(
			dataSystem1.Synchronizers[0].streaming, // first request gets the first stream data
			errorHandler,                           // second request gets the error
			errorHandler,                           // third request also gets the error
			dataSystem2.Synchronizers[0].streaming, // fourth request gets the second stream data
		)
		streamEndpoint := makeStreamEndpoint(t, handler)
		t.Defer(streamEndpoint.Close)

		client := NewSDKClient(t, WithStreamingSynchronizer(baseStreamConfig(streamEndpoint)))
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

	// extendedRegimeConnectionTimeout is used when waiting for a reconnect during extended-regime
	// backoff. Set generously above the compressed ExtendedInitialDelayMS (500 ms) plus any
	// doubling to tolerate CI variance.
	extendedRegimeConnectionTimeout := time.Second * 3

	// The following retry-conformance tests are guarded by CapabilityRetryConformanceFDv1Streaming
	// being PRESENT. They assert RETRY-conformant behavior: unexpected HTTP errors trigger an
	// extended-regime backoff (retry with a longer delay) rather than a permanent stop.
	t.Run("retry after unexpected HTTP error on initial connect", func(t *ldtest.T) {
		t.RequireCapability(servicedef.CapabilityRetryConformanceFDv1Streaming)
		for _, status := range unexpectedErrors {
			t.Run(fmt.Sprintf("error %d", status), func(t *ldtest.T) {
				dataSystem := NewSDKDataSystemWithoutEndpoints(t, dataV1)
				handler := httphelpers.SequentialHandler(
					httphelpers.HandlerWithStatus(status), // 1st: error
					httphelpers.HandlerWithStatus(status), // 2nd: error
					dataSystem.Synchronizers[0].streaming, // 3rd: success
				)
				streamEndpoint := makeStreamEndpoint(t, handler)
				t.Defer(streamEndpoint.Close)

				client := NewSDKClient(t, WithStreamingSynchronizer(retryConformanceStreamConfig(streamEndpoint)))
				result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{Context: o.Some(context)})
				m.In(t).Assert(result, EvalAllFlagsValueForKeyShouldEqual(flagKey, expectedValueV1))

				for i := 0; i < 3; i++ { // expect three requests
					_ = streamEndpoint.RequireConnection(t, extendedRegimeConnectionTimeout)
				}
			})
		}
	})

	t.Run("retry after unexpected HTTP error on reconnect", func(t *ldtest.T) {
		t.RequireCapability(servicedef.CapabilityRetryConformanceFDv1Streaming)
		for _, status := range unexpectedErrors {
			t.Run(fmt.Sprintf("error %d", status), func(t *ldtest.T) {
				dataSystem1 := NewSDKDataSystemWithoutEndpoints(t, dataV1)
				dataSystem2 := NewSDKDataSystemWithoutEndpoints(t, dataV2)
				handler := httphelpers.SequentialHandler(
					dataSystem1.Synchronizers[0].streaming, // 1st: first stream data
					httphelpers.HandlerWithStatus(status),  // 2nd: error
					httphelpers.HandlerWithStatus(status),  // 3rd: error
					dataSystem2.Synchronizers[0].streaming, // 4th: second stream data
				)
				streamEndpoint := makeStreamEndpoint(t, handler)
				t.Defer(streamEndpoint.Close)

				client := NewSDKClient(t, WithStreamingSynchronizer(retryConformanceStreamConfig(streamEndpoint)))
				result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{Context: o.Some(context)})
				m.In(t).Assert(result, EvalAllFlagsValueForKeyShouldEqual(flagKey, expectedValueV1))

				// Cause the initial stream to close; triggers reconnect
				request1 := streamEndpoint.RequireConnection(t, incomingConnectionTimeout)
				request1.Cancel()

				// Two error responses, then a successful reconnect at extended-regime timing
				_ = streamEndpoint.RequireConnection(t, extendedRegimeConnectionTimeout) // 2nd (error)
				_ = streamEndpoint.RequireConnection(t, extendedRegimeConnectionTimeout) // 3rd (error)
				_ = streamEndpoint.RequireConnection(t, extendedRegimeConnectionTimeout) // 4th (success)

				pollUntilFlagValueUpdated(t, client, flagKey, context, expectedValueV1, expectedValueV2, ldvalue.Null())
			})
		}
	})

	t.Run("enters extended-regime backoff after unexpected HTTP error", func(t *ldtest.T) {
		// Paired-bound assertion: after an unexpected error, the SDK MUST NOT retry at
		// normal-regime timing (which would be ~briefDelay = 1 ms), AND MUST eventually retry
		// (proving it hasn't permanently stopped). The 100 ms "no more connections" window is
		// well above briefDelay (100x) and well below compressedExtendedInitialDelay (20% of
		// 500 ms) -- a clean discriminating window.
		t.RequireCapability(servicedef.CapabilityRetryConformanceFDv1Streaming)
		dataSystem := NewSDKDataSystemWithoutEndpoints(t, dataV1)
		handler := httphelpers.SequentialHandler(
			httphelpers.HandlerWithStatus(401), // 1st: unexpected error
			dataSystem.Synchronizers[0].streaming, // 2nd: success (only reached if SDK retries)
		)
		streamEndpoint := makeStreamEndpoint(t, handler)
		t.Defer(streamEndpoint.Close)

		_ = NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{InitCanFail: true}),
			WithStreamingSynchronizer(retryConformanceStreamConfig(streamEndpoint)))

		// 1st connection produces the 401
		_ = streamEndpoint.RequireConnection(t, incomingConnectionTimeout)

		// Assertion A: SDK does NOT reconnect within the normal-regime window (100 ms).
		// If the SDK were retrying at normal-regime timing (1 ms) it would already have reconnected.
		streamEndpoint.RequireNoMoreConnections(t, noMoreConnectionsTimeout)

		// Assertion B: SDK DOES reconnect within the extended-regime window (3 s).
		// If the SDK had permanently stopped, this would fail.
		_ = streamEndpoint.RequireConnection(t, extendedRegimeConnectionTimeout)
	})

	t.Run("does not permanently stop under sustained unexpected HTTP errors", func(t *ldtest.T) {
		// Endpoint returns 401 to every request. SDK should keep retrying at extended-regime
		// cadence. Assert multiple connections observed within a bounded window (proves
		// repeated retry) and one more still arrives at the end (proves not permanently stopped).
		t.RequireCapability(servicedef.CapabilityRetryConformanceFDv1Streaming)
		streamEndpoint := makeStreamEndpoint(t, httphelpers.HandlerWithStatus(401))
		t.Defer(streamEndpoint.Close)

		_ = NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{InitCanFail: true}),
			WithStreamingSynchronizer(retryConformanceStreamConfig(streamEndpoint)))

		// Observe at least 3 connections at extended-regime timing. Each takes ~500 ms; with
		// doubling, total ~3.5 s in the worst case. Generous per-connection timeout.
		for i := 0; i < 3; i++ {
			_ = streamEndpoint.RequireConnection(t, extendedRegimeConnectionTimeout)
		}
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
				dataSystem := NewSDKDataSystemWithoutEndpoints(t, dataV1)
				handler := httphelpers.SequentialHandler(
					// first request gets the error
					httphelpers.HandlerWithStatus(status),
					// second request would succeed and get the stream, but shouldn't happen
					dataSystem.Synchronizers[0].streaming,
				)
				streamEndpoint := makeStreamEndpoint(t, handler)
				t.Defer(streamEndpoint.Close)

				_ = NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{InitCanFail: true}),
					WithStreamingSynchronizer(baseStreamConfig(streamEndpoint)))

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
				dataSystem := NewSDKDataSystemWithoutEndpoints(t, dataV1)
				handler := httphelpers.SequentialHandler(
					// first request gets the stream data
					dataSystem.Synchronizers[0].streaming,
					// second request gets the error
					httphelpers.HandlerWithStatus(status),
					// third request would get the stream again, but shouldn't happen
					dataSystem.Synchronizers[0].streaming,
				)
				streamEndpoint := makeStreamEndpoint(t, handler)
				t.Defer(streamEndpoint.Close)

				client := NewSDKClient(t, WithStreamingSynchronizer(baseStreamConfig(streamEndpoint)))
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
