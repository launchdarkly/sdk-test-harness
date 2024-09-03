package sdktests

import (
	"encoding/json"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

func doServerSideStreamValidationTests(t *ldtest.T) {
	expectedValueV1 := ldvalue.Int(1)
	expectedValueV2 := ldvalue.Int(2)
	flagKey := "flag"
	flagV1, flagV2 := makeFlagVersionsWithValues(flagKey, 1, 2, expectedValueV1, expectedValueV2)
	dataV1 := mockld.NewServerSDKDataBuilder().Flag(flagV1).Build()
	dataV2 := mockld.NewServerSDKDataBuilder().Flag(flagV2).Build()
	context := ldcontext.New("user-key")

	shouldDropAndReconnectAfterEvent := func(t *ldtest.T, badEventName string, badEventData json.RawMessage) {
		stream1 := NewSDKDataSourceWithoutEndpoint(t, dataV1)
		stream2 := NewSDKDataSourceWithoutEndpoint(t, dataV2)
		handler := httphelpers.SequentialHandler(
			stream1.Handler(), // first request gets the first stream data
			stream2.Handler(), // second request gets the second stream data
		)
		streamEndpoint := requireContext(t).harness.NewMockEndpoint(handler, t.DebugLogger(),
			harness.MockEndpointDescription("streaming service"))
		t.Defer(streamEndpoint.Close)

		client := NewSDKClient(t, WithStreamingConfig(baseStreamConfig(streamEndpoint)))
		result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{Context: o.Some(context)})
		m.In(t).Assert(result, EvalAllFlagsValueForKeyShouldEqual(flagKey, expectedValueV1))

		// Get & discard the request info for the first request
		_ = streamEndpoint.RequireConnection(t, time.Second*5)

		// Send the bad event; this should cause the SDK to drop the first stream
		stream1.StreamingService().PushEvent(badEventName, badEventData)

		// Expect the second request; it succeeds and gets the second stream data
		_ = streamEndpoint.RequireConnection(t, time.Second*5)

		// Check that the client got the new data from the second stream
		pollUntilFlagValueUpdated(t, client, flagKey, context, expectedValueV1, expectedValueV2, ldvalue.Null())
	}

	t.Run("drop and reconnect if stream event has malformed JSON", func(t *ldtest.T) {
		t.Run("server-intent event", func(t *ldtest.T) {
			shouldDropAndReconnectAfterEvent(t, "server-intent", []byte(`{sorry`))
		})

		// TODO: Update these tests. We aren't decoding the JSON as early, so the behavior might not be right here, or it might not be right in the SDK. need to investigatright in the SDK. Need to investigate.
		// t.Run("put-object event", func(t *ldtest.T) {
		// 	shouldDropAndReconnectAfterEvent(t, "put-object", []byte(`{sorry`))
		// })

		// TODO: Update these tests. We aren't decoding the JSON as early, so the behavior might not be right here, or it might not be right in the SDK. need to investigatright in the SDK. Need to investigate.
		// t.Run("delete event", func(t *ldtest.T) {
		// 	shouldDropAndReconnectAfterEvent(t, "delete-object", []byte(`{sorry`))
		// })

		// TODO: Add more fdv2 event types
	})

	t.Run("drop and reconnect if stream event has well-formed JSON not matching schema", func(t *ldtest.T) {
		t.Run("server-intent event", func(t *ldtest.T) {
			shouldDropAndReconnectAfterEvent(t, "server-intent", []byte(`{"data":{"flags": true, "segments":{}}}`))
		})

		// TODO: Update these tests. We aren't decoding the JSON as early, so the behavior might not be right here, or it might not be right in the SDK. need to investigatright in the SDK. Need to investigate.
		// t.Run("put event", func(t *ldtest.T) {
		// 	shouldDropAndReconnectAfterEvent(t, "put-object", []byte(`{"path":"/flags/x","data":true}`))
		// })

		// TODO: Update these tests. We aren't decoding the JSON as early, so the behavior might not be right here, or it might not be right in the SDK. need to investigatright in the SDK. Need to investigate.
		// t.Run("delete event", func(t *ldtest.T) {
		// 	shouldDropAndReconnectAfterEvent(t, "delete-object", []byte(`{"path":"/flags/x","version":"no"`))
		// })

		// TODO: Add more fdv2 event types
	})

	shouldIgnoreEvent := func(t *ldtest.T, eventName string, eventData json.RawMessage) {
		dataSource := NewSDKDataSource(t, dataV1)
		client := NewSDKClient(t, WithStreamingConfig(servicedef.SDKConfigStreamingParams{
			InitialRetryDelayMS: o.Some(briefDelay), // brief delay so we can easily detect if it reconnects
		}), dataSource)

		result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{Context: o.Some(context)})
		m.In(t).Assert(result, EvalAllFlagsValueForKeyShouldEqual(flagKey, expectedValueV1))

		// Get & discard the request info for the first request
		_ = dataSource.Endpoint().RequireConnection(t, time.Second*5)

		// Push an event that isn't recognized, but isn't bad enough to cause any problems
		dataSource.StreamingService().PushEvent(eventName, eventData)

		// Then, push a patch event, so we can detect if the SDK continued processing the stream as it should
		dataSource.StreamingService().PushUpdate("flag", flagKey, flagV2.Version, jsonhelpers.ToJSON(flagV2))
		// TODO: Need to determine which version this should be, and also what the state should be
		dataSource.StreamingService().PushPayloadTransferred("state", 2)

		// Check that the client got the new data
		pollUntilFlagValueUpdated(t, client, flagKey, context, expectedValueV1, expectedValueV2, ldvalue.Null())

		// Verify that it did not reconnect
		dataSource.Endpoint().RequireNoMoreConnections(t, time.Millisecond*100)
	}

	t.Run("unrecognized data that can be safely ignored", func(t *ldtest.T) {
		// SDKs are required to be tolerant of some kinds of unrecognized data, to leave room for future
		// expansion.

		t.Run("unknown event name with JSON body", func(t *ldtest.T) {
			shouldIgnoreEvent(t, "whatever", []byte(`{}`))
		})

		t.Run("unknown event name with non-JSON body", func(t *ldtest.T) {
			// The SDK shouldn't try to parse the data field at all for an unknown event type
			shouldIgnoreEvent(t, "whatever", []byte(`not JSON`))
		})

		t.Run("patch event with unrecognized path kind", func(t *ldtest.T) {
			shouldIgnoreEvent(t, "patch", []byte(`{"path": "/cats/Jack", "data": {}}`))
		})
	})
}
