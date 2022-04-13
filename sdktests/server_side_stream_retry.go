package sdktests

import (
	"fmt"
	"net/http"
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

const briefDelay ldtime.UnixMillisecondTime = 1

func baseStreamConfig(endpoint *harness.MockEndpoint) servicedef.SDKConfigStreamingParams {
	return servicedef.SDKConfigStreamingParams{
		BaseURI:             endpoint.BaseURL(),
		InitialRetryDelayMs: o.Some(briefDelay),
	}
}

func doServerSideStreamRetryTests(t *ldtest.T) {
	recoverableErrors := []int{400, 408, 429, 500, 503}
	unrecoverableErrors := []int{401, 403, 405} // really all 4xx errors that aren't 400, 408, or 429

	expectedValueV1 := ldvalue.Int(1)
	expectedValueV2 := ldvalue.Int(2)
	flagKey := "flag"
	flagV1, flagV2 := makeFlagVersionsWithValues(flagKey, 1, 2, expectedValueV1, expectedValueV2)
	dataV1 := mockld.NewServerSDKDataBuilder().Flag(flagV1).Build()
	dataV2 := mockld.NewServerSDKDataBuilder().Flag(flagV2).Build()
	user := lduser.NewUser("user-key")

	t.Run("retry after stream is closed", func(t *ldtest.T) {
		stream1 := NewSDKDataSourceWithoutEndpoint(t, dataV1)
		stream2 := NewSDKDataSourceWithoutEndpoint(t, dataV2)
		handler := httphelpers.SequentialHandler(
			stream1.Handler(), // first request gets the first stream data
			stream2.Handler(), // second request gets the second stream data
		)
		streamEndpoint := requireContext(t).harness.NewMockEndpoint(handler, nil, t.DebugLogger())
		t.Defer(streamEndpoint.Close)

		client := NewSDKClient(t, WithStreamingConfig(baseStreamConfig(streamEndpoint)))
		result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{User: o.Some(user)})
		m.In(t).Assert(result, EvalAllFlagsValueForKeyShouldEqual(flagKey, expectedValueV1))

		// Get the request info for the first request
		request1 := streamEndpoint.RequireConnection(t, time.Second*5)

		// Now cause the stream to close; this should trigger a reconnect
		request1.Cancel()

		// Expect the second request; it succeeds and gets the second stream data
		_ = streamEndpoint.RequireConnection(t, time.Millisecond*100)

		// Check that the client got the new data from the second stream
		pollUntilFlagValueUpdated(t, client, flagKey, user, expectedValueV1, expectedValueV2, ldvalue.Null())
	})

	t.Run("initial retry delay is applied", func(t *ldtest.T) {
		// Since execution time in a test environment is highly unpredictable, we can't really make
		// expectations about seeing specific retry delays. But we can at least verify that if we set
		// the initial delay to a very large value, we should not see a reconnection attempt within a
		// short time.

		stream := NewSDKDataSource(t, dataV1)
		client := NewSDKClient(t,
			WithStreamingConfig(servicedef.SDKConfigStreamingParams{
				InitialRetryDelayMs: o.Some(ldtime.UnixMillisecondTime(10000)),
			}),
			stream,
		)
		result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{User: o.Some(user)})
		m.In(t).Assert(result, EvalAllFlagsValueForKeyShouldEqual(flagKey, expectedValueV1))

		// Get the request info for the first request
		request1 := stream.Endpoint().RequireConnection(t, time.Second*5)

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
		stream.Endpoint().RequireNoMoreConnections(t, time.Millisecond*100)
	})

	shouldRetryAfterErrorOnInitialConnect := func(t *ldtest.T, errorHandler http.Handler) {
		stream := NewSDKDataSourceWithoutEndpoint(t, dataV1)
		handler := httphelpers.SequentialHandler(
			errorHandler,     // first request gets the error
			errorHandler,     // second request also gets the error
			stream.Handler(), // third request succeeds and gets the stream
		)
		streamEndpoint := requireContext(t).harness.NewMockEndpoint(handler, nil, t.DebugLogger())
		t.Defer(streamEndpoint.Close)

		client := NewSDKClient(t, WithStreamingConfig(baseStreamConfig(streamEndpoint)))
		result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{User: o.Some(user)})
		m.In(t).Assert(result, EvalAllFlagsValueForKeyShouldEqual(flagKey, expectedValueV1))

		for i := 0; i < 3; i++ { // expect three requests
			_ = streamEndpoint.RequireConnection(t, time.Second*5)
		}

		streamEndpoint.RequireNoMoreConnections(t, time.Millisecond*100)
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
		streamEndpoint := requireContext(t).harness.NewMockEndpoint(handler, nil, t.DebugLogger())
		t.Defer(streamEndpoint.Close)

		client := NewSDKClient(t, WithStreamingConfig(baseStreamConfig(streamEndpoint)))
		result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{User: o.Some(user)})
		m.In(t).Assert(result, EvalAllFlagsValueForKeyShouldEqual(flagKey, expectedValueV1))

		// Get the request info for the first request
		request1 := streamEndpoint.RequireConnection(t, time.Second*5)

		// Now cause the stream to close; this should trigger a reconnect
		request1.Cancel()

		// Expect the second request; it will receive an error, causing another attempt
		_ = streamEndpoint.RequireConnection(t, time.Millisecond*100)

		// Expect the third request; it will also receive an error, causing another attempt
		_ = streamEndpoint.RequireConnection(t, time.Millisecond*100)

		// expect the fourth request; this one succeeds and gets the second stream data
		_ = streamEndpoint.RequireConnection(t, time.Millisecond*100)

		// check that the client got the new data from the second stream
		pollUntilFlagValueUpdated(t, client, flagKey, user, expectedValueV1, expectedValueV2, ldvalue.Null())
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

	t.Run("do not retry after unrecoverable HTTP error on initial connect", func(t *ldtest.T) {
		for _, status := range unrecoverableErrors {
			t.Run(fmt.Sprintf("error %d", status), func(t *ldtest.T) {
				stream := NewSDKDataSourceWithoutEndpoint(t, dataV1)
				handler := httphelpers.SequentialHandler(
					httphelpers.HandlerWithStatus(status), // first request gets the error
					stream.Handler(),                      // second request would succeed and get the stream, but shouldn't happen
				)
				streamEndpoint := requireContext(t).harness.NewMockEndpoint(handler, nil, t.DebugLogger())
				t.Defer(streamEndpoint.Close)

				_ = NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{InitCanFail: true}),
					WithStreamingConfig(baseStreamConfig(streamEndpoint)))

				_ = streamEndpoint.RequireConnection(t, time.Second*5)

				streamEndpoint.RequireNoMoreConnections(t, time.Millisecond*100)
			})
		}
	})

	t.Run("do not retry after unrecoverable HTTP error on reconnect", func(t *ldtest.T) {
		for _, status := range unrecoverableErrors {
			t.Run(fmt.Sprintf("error %d", status), func(t *ldtest.T) {
				stream := NewSDKDataSourceWithoutEndpoint(t, dataV1)
				handler := httphelpers.SequentialHandler(
					stream.Handler(),                      // first request gets the stream data
					httphelpers.HandlerWithStatus(status), // second request gets the error
					stream.Handler(),                      // third request would get the stream again, but shouldn't happen
				)
				streamEndpoint := requireContext(t).harness.NewMockEndpoint(handler, nil, t.DebugLogger())
				t.Defer(streamEndpoint.Close)

				client := NewSDKClient(t, WithStreamingConfig(baseStreamConfig(streamEndpoint)))
				result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{User: o.Some(user)})
				m.In(t).Assert(result, EvalAllFlagsValueForKeyShouldEqual(flagKey, expectedValueV1))

				// get the request info for the first request
				request1 := streamEndpoint.RequireConnection(t, time.Second*5)

				// now cause the stream to close; this should trigger a reconnect
				request1.Cancel()

				// expect the second request; it will receive an error
				_ = streamEndpoint.RequireConnection(t, time.Millisecond*100)

				streamEndpoint.RequireNoMoreConnections(t, time.Millisecond*100)
			})
		}
	})
}
