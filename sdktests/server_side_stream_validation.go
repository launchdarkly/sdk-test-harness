package sdktests

import (
	"encoding/json"
	"time"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

func doServerSideStreamValidationTests(t *ldtest.T) {
	expectedValueV1 := ldvalue.Int(1)
	expectedValueV2 := ldvalue.Int(2)
	flagKey := "flag"
	flagV1, flagV2 := makeFlagVersionsWithValues(flagKey, 1, 2, expectedValueV1, expectedValueV2)
	dataV1 := mockld.NewServerSDKDataBuilder().Flag(flagV1).Build()
	dataV2 := mockld.NewServerSDKDataBuilder().Flag(flagV2).Build()
	user := lduser.NewUser("user-key")

	shouldDropAndReconnectAfterEvent := func(t *ldtest.T, badEventName string, badEventData json.RawMessage) {
		stream1 := NewSDKDataSourceWithoutEndpoint(t, dataV1)
		stream2 := NewSDKDataSourceWithoutEndpoint(t, dataV2)
		handler := httphelpers.SequentialHandler(
			stream1.Handler(), // first request gets the first stream data
			stream2.Handler(), // second request gets the second stream data
		)
		streamEndpoint := requireContext(t).harness.NewMockEndpoint(handler, nil, t.DebugLogger())
		t.Defer(streamEndpoint.Close)

		client := NewSDKClient(t,
			WithConfig(servicedef.SDKConfigParams{
				Streaming: baseStreamConfig(streamEndpoint),
			}),
		)
		result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{User: &user})
		m.In(t).Assert(result, EvalAllFlagsValueForKeyShouldEqual(flagKey, expectedValueV1))

		// Get & discard the request info for the first request
		_ = expectRequest(t, streamEndpoint, time.Second*5)

		// Send the bad event; this should cause the SDK to drop the first stream
		stream1.Service().PushEvent(badEventName, badEventData)

		// Expect the second request; it succeeds and gets the second stream data
		_ = expectRequest(t, streamEndpoint, time.Millisecond*100)

		// Check that the client got the new data from the second stream
		pollUntilFlagValueUpdated(t, client, flagKey, user, expectedValueV1, expectedValueV2, ldvalue.Null())
	}

	t.Run("drop and reconnect if stream event has malformed JSON", func(t *ldtest.T) {
		t.Run("put event", func(t *ldtest.T) {
			shouldDropAndReconnectAfterEvent(t, "put", []byte(`{sorry`))
		})

		t.Run("patch event", func(t *ldtest.T) {
			shouldDropAndReconnectAfterEvent(t, "patch", []byte(`{sorry`))
		})

		t.Run("delete event", func(t *ldtest.T) {
			shouldDropAndReconnectAfterEvent(t, "delete", []byte(`{sorry`))
		})
	})

	t.Run("drop and reconnect if stream event has well-formed JSON not matching schema", func(t *ldtest.T) {
		t.Run("put event", func(t *ldtest.T) {
			shouldDropAndReconnectAfterEvent(t, "put", []byte(`{"data":{"flags": true, "segments":{}}}`))
		})

		t.Run("patch event", func(t *ldtest.T) {
			shouldDropAndReconnectAfterEvent(t, "patch", []byte(`{"path":"/flags/x","data":true}`))
		})

		t.Run("delete event", func(t *ldtest.T) {
			shouldDropAndReconnectAfterEvent(t, "delete", []byte(`{"path":"/flags/x","version":"no"`))
		})
	})

	shouldIgnoreEvent := func(t *ldtest.T, eventName string, eventData json.RawMessage) {
		dataSource := NewSDKDataSource(t, dataV1)
		client := NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{
			Streaming: &servicedef.SDKConfigStreamingParams{
				InitialRetryDelayMs: timeValueAsPointer(briefDelay), // brief delay so we can easily detect if it reconnects
			},
		}), dataSource)

		result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{User: &user})
		m.In(t).Assert(result, EvalAllFlagsValueForKeyShouldEqual(flagKey, expectedValueV1))

		// Get & discard the request info for the first request
		_ = expectRequest(t, dataSource.Endpoint(), time.Second*5)

		// Push an event that isn't recognized, but isn't bad enough to cause any problems
		dataSource.Service().PushEvent(eventName, eventData)

		// Then, push a patch event, so we can detect if the SDK continued processing the stream as it should
		dataSource.Service().PushUpdate("flags", flagKey, jsonhelpers.ToJSON(flagV2))

		// Check that the client got the new data
		pollUntilFlagValueUpdated(t, client, flagKey, user, expectedValueV1, expectedValueV2, ldvalue.Null())

		// Verify that it did not reconnect
		expectNoMoreRequests(t, dataSource.Endpoint())
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