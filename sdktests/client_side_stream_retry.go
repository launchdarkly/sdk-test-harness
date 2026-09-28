package sdktests

import (
	"fmt"
	"net/http"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v3/framework/harness"
	h "github.com/launchdarkly/sdk-test-harness/v3/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

func doClientSideStreamRetryTests(t *ldtest.T) {
	c := NewCommonStreamingTests(t, "doClientSideStreamRetryTests")

	recoverableErrors := []int{400, 408, 429, 500, 503}
	unrecoverableErrors := []int{401, 403, 405} // really all 4xx errors that aren't 400, 408, or 429

	expectedValueV1 := ldvalue.Int(1)
	expectedValueV2 := ldvalue.Int(2)
	flagKey := "flag-key" // the key used by CommonStreamingTests.makeSDKDataWithFlag
	dataV1 := c.makeSDKDataWithFlag(1, expectedValueV1)
	dataV2 := c.makeSDKDataWithFlag(2, expectedValueV2)
	context := ldcontext.New("user-key")

	// Because we're setting InitialRetryDelayMS to a very short delay, we expect reconnections to
	// happen quickly - but, execution speed is always unpredictable, so we'll use a timeout for
	// these that is much longer than we expect we'll need. That won't make the tests run any slower
	// than they otherwise would unless the SDK really is hanging and not reconnecting.
	//
	// This timeout is deliberately more generous than the server-side equivalent: not every
	// client-side SDK supports configuring the stream retry delay, and with a default initial
	// delay of about a second, exponential backoff can put the third retry several seconds out.
	incomingConnectionTimeout := time.Second * 10

	// When we're asserting "there are no more connections", we should use a timeout that isn't too
	// long because that *will* make successful tests run slow, but long enough that we have a
	// reasonable chance of detecting an inappropriate retry that happened promptly.
	noMoreConnectionsTimeout := time.Millisecond * 100

	// Creates a mock streaming endpoint with the given handler and configures it as the SDK's only
	// streaming synchronizer. As in CommonStreamingTests.setupDataSystems, no polling initializer is
	// configured, so the SDK starts from an empty basis and gets all of its data from the stream.
	makeStreamEndpointAndConfigurers := func(t *ldtest.T, handler http.Handler) (*harness.MockEndpoint, []SDKConfigurer) {
		streamEndpoint := requireContext(t).harness.NewMockEndpoint(handler, t.DebugLogger(),
			harness.MockEndpointDescription("streaming service"))
		t.Defer(streamEndpoint.Close)

		return streamEndpoint, []SDKConfigurer{WithStreamingSynchronizer(baseStreamConfig(streamEndpoint))}
	}

	newStreamingService := func(t *ldtest.T, data mockld.SDKData) http.Handler {
		return mockld.NewStreamingService(data, c.sdkKind, t.DebugLogger())
	}

	t.Run("retry after stream is closed", func(t *ldtest.T) {
		stream1 := newStreamingService(t, dataV1)
		stream2 := newStreamingService(t, dataV2)
		handler := httphelpers.SequentialHandler(
			stream1, // first request gets the first stream data
			stream2, // second request gets the second stream data
		)
		streamEndpoint, configurers := makeStreamEndpointAndConfigurers(t, handler)

		client := NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)
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

	// Client-side SDKs generally resolve their initialization as soon as the first data source
	// connection attempt completes, even if the failure is one that the data source will retry -
	// blocking application startup on retries would not be appropriate on a mobile device. So,
	// unlike the equivalent server-side tests, we create the client with InitCanFail and then
	// verify that the SDK keeps retrying in the background and eventually gets the flag data.
	//
	// No polling initializer is configured, so the SDK starts with no flag data; seeing
	// expectedValueV2 proves the data came from the successful stream connection.
	shouldRetryAfterErrorOnInitialConnect := func(t *ldtest.T, errorHandler http.Handler) {
		stream := newStreamingService(t, dataV2)
		handler := httphelpers.SequentialHandler(
			errorHandler, // first request gets the error
			errorHandler, // second request also gets the error
			stream,       // third request succeeds and gets the stream
		)
		streamEndpoint, configurers := makeStreamEndpointAndConfigurers(t, handler)

		client := NewSDKClient(t, append(
			[]SDKConfigurer{WithConfig(servicedef.SDKConfigParams{InitCanFail: true})},
			c.baseSDKConfigurationPlus(configurers...)...)...)

		for i := 0; i < 3; i++ { // expect three requests
			_ = streamEndpoint.RequireConnection(t, incomingConnectionTimeout)
		}

		streamEndpoint.RequireNoMoreConnections(t, noMoreConnectionsTimeout)

		// The third connection got the stream data, so the flag value should show up promptly
		h.RequireEventually(t, func() bool {
			return basicEvaluateFlag(t, client, flagKey, context, ldvalue.Null()).Equal(expectedValueV2)
		}, time.Second*5, time.Millisecond*50, "timed out without seeing flag data from the stream")
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
		stream1 := newStreamingService(t, dataV1)
		stream2 := newStreamingService(t, dataV2)
		handler := httphelpers.SequentialHandler(
			stream1,      // first request gets the first stream data
			errorHandler, // second request gets the error
			errorHandler, // third request also gets the error
			stream2,      // fourth request gets the second stream data
		)
		streamEndpoint, configurers := makeStreamEndpointAndConfigurers(t, handler)

		client := NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)
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

		// Expect the fourth request; this one succeeds and gets the second stream data
		_ = streamEndpoint.RequireConnection(t, incomingConnectionTimeout)

		// Check that the client got the new data from the second stream
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

	t.Run("do not retry after unrecoverable HTTP error on initial connect", func(t *ldtest.T) {
		// Distinguishing unrecoverable HTTP errors requires the SDK's EventSource to expose
		// response status codes; browser-native EventSource cannot, so SDKs without this
		// capability treat every stream error as retryable.
		t.RequireCapability(servicedef.CapabilityClientEventSourceHTTPErrors)
		for _, status := range unrecoverableErrors {
			t.Run(fmt.Sprintf("error %d", status), func(t *ldtest.T) {
				stream := newStreamingService(t, dataV1)
				handler := httphelpers.SequentialHandler(
					httphelpers.HandlerWithStatus(status), // first request gets the error
					stream,                                // second request would succeed and get the stream, but shouldn't happen
				)
				streamEndpoint, configurers := makeStreamEndpointAndConfigurers(t, handler)

				_ = NewSDKClient(t, append(
					[]SDKConfigurer{WithConfig(servicedef.SDKConfigParams{InitCanFail: true})},
					c.baseSDKConfigurationPlus(configurers...)...)...)

				_ = streamEndpoint.RequireConnection(t, incomingConnectionTimeout)

				streamEndpoint.RequireNoMoreConnections(t, noMoreConnectionsTimeout)
			})
		}
	})

	t.Run("do not retry after unrecoverable HTTP error on reconnect", func(t *ldtest.T) {
		t.RequireCapability(servicedef.CapabilityClientEventSourceHTTPErrors)
		for _, status := range unrecoverableErrors {
			t.Run(fmt.Sprintf("error %d", status), func(t *ldtest.T) {
				stream := newStreamingService(t, dataV1)
				handler := httphelpers.SequentialHandler(
					stream,                                // first request gets the stream data
					httphelpers.HandlerWithStatus(status), // second request gets the error
					stream,                                // third request would get the stream again, but shouldn't happen
				)
				streamEndpoint, configurers := makeStreamEndpointAndConfigurers(t, handler)

				client := NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)
				result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{Context: o.Some(context)})
				m.In(t).Assert(result, EvalAllFlagsValueForKeyShouldEqual(flagKey, expectedValueV1))

				// Get the request info for the first request
				request1 := streamEndpoint.RequireConnection(t, incomingConnectionTimeout)

				// Now cause the stream to close; this should trigger a reconnect
				request1.Cancel()

				// Expect the second request; it will receive an error
				_ = streamEndpoint.RequireConnection(t, incomingConnectionTimeout)

				streamEndpoint.RequireNoMoreConnections(t, noMoreConnectionsTimeout)
			})
		}
	})
}
