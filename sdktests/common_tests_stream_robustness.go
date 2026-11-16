package sdktests

import (
	"encoding/json"
	"time"

	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makePutEventData returns the data of a stream put event that replaces all data with the given data.
func (c CommonStreamingTests) makePutEventData(data mockld.SDKData) json.RawMessage {
	if c.isClientSide {
		return data.Serialize()
	}
	return jsonhelpers.ToJSON(map[string]interface{}{"path": "/", "data": json.RawMessage(data.Serialize())})
}

// PutReplacesAllData verifies that a "put" event received in the middle of a stream replaces the SDK's
// entire data set, rather than being merged into it: flags and segments that are not in the new data
// set no longer exist.
func (c CommonStreamingTests) PutReplacesAllData(t *ldtest.T) {
	t.Run("put event replaces all data", func(t *ldtest.T) {
		context := ldcontext.New("context-key")
		keptFlagKey, removedFlagKey := "kept-flag", "removed-flag"
		segmentFlagKey, segmentKey := "segment-flag", "removed-segment"
		valueBefore, valueAfter := ldvalue.String("value-before"), ldvalue.String("value-after")
		removedValue := ldvalue.String("removed-flag-value")
		notInSegment, inSegment := ldvalue.String("not-in-segment"), ldvalue.String("in-segment")
		defaultValue := ldvalue.String("default")

		var dataBefore, dataAfter mockld.SDKData
		if c.isClientSide {
			dataBefore = mockld.NewClientSDKDataBuilder().
				Flag(keptFlagKey, c.makeClientSideFlag(keptFlagKey, 1, valueBefore).ClientSDKFlag).
				Flag(removedFlagKey, c.makeClientSideFlag(removedFlagKey, 1, removedValue).ClientSDKFlag).
				Build()
			dataAfter = mockld.NewClientSDKDataBuilder().
				Flag(keptFlagKey, c.makeClientSideFlag(keptFlagKey, 2, valueAfter).ClientSDKFlag).
				Build()
		} else {
			segment := ldbuilders.NewSegmentBuilder(segmentKey).Version(1).Included(context.Key()).Build()
			segmentFlag := makeFlagToCheckSegmentMatch(segmentFlagKey, segmentKey, notInSegment, inSegment)
			dataBefore = mockld.NewServerSDKDataBuilder().
				Flag(c.makeServerSideFlag(keptFlagKey, 1, valueBefore), c.makeServerSideFlag(removedFlagKey, 1, removedValue),
					segmentFlag).
				Segment(segment).
				Build()
			dataAfter = mockld.NewServerSDKDataBuilder().
				Flag(c.makeServerSideFlag(keptFlagKey, 2, valueAfter), segmentFlag).
				Build()
		}

		stream, configurers := c.setupDataSources(t, dataBefore)
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)
		_ = requireStreamConnection(t, stream.Endpoint(), time.Second*5)

		m.In(t).Assert(basicEvaluateFlag(t, client, keptFlagKey, context, defaultValue), m.Equal(valueBefore))
		m.In(t).Assert(basicEvaluateFlag(t, client, removedFlagKey, context, defaultValue), m.Equal(removedValue))
		if !c.isClientSide {
			m.In(t).Assert(basicEvaluateFlag(t, client, segmentFlagKey, context, defaultValue), m.Equal(inSegment))
		}

		stream.StreamingService().PushEvent("put", c.makePutEventData(dataAfter))

		pollUntilFlagValueUpdated(t, client, keptFlagKey, context, valueBefore, valueAfter, defaultValue)

		result := evaluateFlagDetail(t, client, removedFlagKey, context, defaultValue)
		m.In(t).For("flag absent from the new put").Assert(result, m.AllOf(
			EvalResponseValue().Should(m.Equal(defaultValue)),
			EvalResponseReason().Should(EqualReason(ldreason.NewEvalReasonError(ldreason.EvalErrorFlagNotFound))),
		))
		allFlags := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{Context: o.Some(context)})
		assert.NotContains(t, allFlags.State, removedFlagKey)

		if !c.isClientSide {
			// A segment that is not in the new data set no longer exists, so the context is not in it.
			m.In(t).For("flag that uses the segment absent from the new put").Assert(
				basicEvaluateFlag(t, client, segmentFlagKey, context, defaultValue), m.Equal(notInSegment))
		}
	})
}

// IgnoresComments verifies that SSE comment lines, which LaunchDarkly sends as keep-alive heartbeats,
// are ignored: they do not cause the SDK to drop the stream, and later events on the same stream are
// still applied.
func (c CommonStreamingTests) IgnoresComments(t *ldtest.T) {
	t.Run("comment lines are ignored", func(t *ldtest.T) {
		context := ldcontext.New("context-key")
		flagKey := "flag"
		valueBefore, valueAfter := ldvalue.String("value-before"), ldvalue.String("value-after")
		defaultValue := ldvalue.String("default")
		data := c.makeSDKDataWithFlag(flagKey, 1, valueBefore)

		// A short retry delay makes a reconnect prompt, so that the check at the end can see it.
		stream, configurers := c.setupDataSources(t, data)
		configurers = append(configurers, WithStreamingConfig(baseStreamConfig(stream.Endpoint())))
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)
		_ = requireStreamConnection(t, stream.Endpoint(), time.Second*5)

		// The initial value proves that the stream subscription exists, so the comments below are not lost.
		h.RequireEventually(t, func() bool {
			return basicEvaluateFlag(t, client, flagKey, context, defaultValue).Equal(valueBefore)
		}, time.Second*5, time.Millisecond*50, "SDK did not receive the initial stream data")

		stream.StreamingService().PushComment("")
		stream.StreamingService().PushComment("heartbeat")
		stream.StreamingService().PushComment(`{"not": "an event"}`)
		stream.StreamingService().PushUpdate("flags", flagKey, c.makeFlagData(flagKey, 2, valueAfter))

		pollUntilFlagValueUpdated(t, client, flagKey, context, valueBefore, valueAfter, defaultValue)
		stream.Endpoint().RequireNoMoreConnections(t, time.Millisecond*100)
	})
}

// NoReconnectAfterClose verifies that once the SDK client has been closed, its stream does not
// reconnect, even if the service then ends the stream from its side.
func (c CommonStreamingTests) NoReconnectAfterClose(t *ldtest.T) {
	t.Run("SDK does not reconnect after the client is closed", func(t *ldtest.T) {
		// A short retry delay makes a reconnect prompt, so that the check at the end can see it.
		stream, configurers := c.setupDataSources(t, nil)
		configurers = append(configurers, WithStreamingConfig(baseStreamConfig(stream.Endpoint())))
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)
		request := requireStreamConnection(t, stream.Endpoint(), time.Second*5)

		// Close the client, wait for the SDK to close the stream, and then end the stream from the
		// service side too.
		require.NoError(t, client.Close())
		h.RequireEventually(t, func() bool {
			select {
			case <-request.Context.Done():
				return true
			default:
				return false
			}
		}, time.Second*3, time.Millisecond*20, "SDK did not close the streaming connection after the client was closed")
		request.Cancel()

		// The SDK must not open a new stream.
		stream.Endpoint().RequireNoMoreConnections(t, time.Second)
	})
}
