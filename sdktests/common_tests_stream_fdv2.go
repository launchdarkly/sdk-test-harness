package sdktests

import (
	"net/http"
	"time"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/stretchr/testify/require"
)

var (
	initialValue = ldvalue.String("initial value") //nolint:gochecknoglobals
	updatedValue = ldvalue.String("updated value") //nolint:gochecknoglobals

	newInitialValue = ldvalue.String("new initial value") //nolint:gochecknoglobals

	defaultValue = ldvalue.String("default value") //nolint:gochecknoglobals
)

func (c CommonStreamingTests) FDv2(t *ldtest.T) {
	t.Run("reconnection state management", c.StateTransitions)
	t.Run(
		"updates are not complete until payload transferred is sent",
		c.UpdatesAreNotCompleteUntilPayloadTransferredIsSent)
	t.Run("ignores model version", c.IgnoresModelVersion)
	t.Run("ignores heart beat", c.IgnoresHeartBeat)
	t.Run("discards events on errors", c.DiscardsEventsOnError)
	t.Run("disconnects on goodbye", c.DisconnectsOnGoodbye)
}

func (c CommonStreamingTests) StateTransitions(t *ldtest.T) {
	t.Run("initializes from an empty state", c.InitializeFromEmptyState)
	t.Run("saves previously known state", c.SavesPreviouslyKnownState)
	t.Run("replaces previously known state", c.ReplacesPreviouslyKnownState)
	t.Run("updates previously known state", c.UpdatesPreviouslyKnownState)
}

func (c CommonStreamingTests) InitializeFromEmptyState(t *ldtest.T) {
	streamEndpoint, _ := makeSequentialStreamHandler(t, c.makeSDKDataWithFlag("flag-key", 1, initialValue))
	t.Defer(streamEndpoint.Close)
	client := NewSDKClient(t, WithStreamingConfig(baseStreamConfig(streamEndpoint)))

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue}
	validatePayloadReceived(t, streamEndpoint, client, "", expectedEvaluations)
}

func (c CommonStreamingTests) SavesPreviouslyKnownState(t *ldtest.T) {
	dataBefore := c.makeSDKDataWithFlag("flag-key", 1, initialValue)
	dataAfter := mockld.NewServerSDKDataBuilder().IntentCode("xfer-none").IntentReason("up-to-date").Build()
	streamEndpoint, _ := makeSequentialStreamHandler(t, dataBefore, dataAfter)
	t.Defer(streamEndpoint.Close)
	client := NewSDKClient(t, WithStreamingConfig(baseStreamConfig(streamEndpoint)))

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue}
	request := validatePayloadReceived(t, streamEndpoint, client, "", expectedEvaluations)
	request.Cancel() // Drop the stream and allow the SDK to reconnect

	validatePayloadReceived(t, streamEndpoint, client, "initial", expectedEvaluations)
}

func (c CommonStreamingTests) ReplacesPreviouslyKnownState(t *ldtest.T) {
	dataBefore := c.makeSDKDataWithFlag("flag-key", 1, initialValue)
	dataAfter := mockld.NewServerSDKDataBuilder().
		IntentCode("xfer-full").
		IntentReason("cant-catchup").
		Flag(c.makeServerSideFlag("new-flag-key", 1, ldvalue.String("replacement value"))).
		Build()
	streamEndpoint, _ := makeSequentialStreamHandler(t, dataBefore, dataAfter)
	t.Defer(streamEndpoint.Close)
	client := NewSDKClient(t, WithStreamingConfig(baseStreamConfig(streamEndpoint)))

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue, "new-flag-key": defaultValue}
	request := validatePayloadReceived(t, streamEndpoint, client, "", expectedEvaluations)
	request.Cancel() // Drop the stream and allow the SDK to reconnect

	expectedEvaluations = map[string]ldvalue.Value{
		"flag-key":     defaultValue,
		"new-flag-key": ldvalue.String("replacement value")}
	validatePayloadReceived(t, streamEndpoint, client, "initial", expectedEvaluations)
}

func (c CommonStreamingTests) UpdatesPreviouslyKnownState(t *ldtest.T) {
	dataBefore := c.makeSDKDataWithFlag("flag-key", 1, initialValue)
	dataAfter := mockld.NewServerSDKDataBuilder().
		IntentCode("xfer-changes").
		IntentReason("stale").
		Flag(c.makeServerSideFlag("flag-key", 2, updatedValue)).
		Flag(c.makeServerSideFlag("new-flag-key", 1, newInitialValue)).
		Build()
	streamEndpoint, _ := makeSequentialStreamHandler(t, dataBefore, dataAfter)
	t.Defer(streamEndpoint.Close)
	client := NewSDKClient(t, WithStreamingConfig(baseStreamConfig(streamEndpoint)))

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue, "new-flag-key": defaultValue}
	request := validatePayloadReceived(t, streamEndpoint, client, "", expectedEvaluations)
	request.Cancel() // Drop the stream and allow the SDK to reconnect

	expectedEvaluations = map[string]ldvalue.Value{"flag-key": updatedValue, "new-flag-key": newInitialValue}
	validatePayloadReceived(t, streamEndpoint, client, "initial", expectedEvaluations)
}

func (c CommonStreamingTests) UpdatesAreNotCompleteUntilPayloadTransferredIsSent(t *ldtest.T) {
	data := c.makeSDKDataWithFlag("flag-key", 1, initialValue)
	stream := NewSDKDataSourceWithoutEndpoint(t, data)
	streamEndpoint := requireContext(t).harness.NewMockEndpoint(stream.Handler(), t.DebugLogger(),
		harness.MockEndpointDescription("streaming service"))
	t.Defer(streamEndpoint.Close)
	client := NewSDKClient(t, WithStreamingConfig(baseStreamConfig(streamEndpoint)))

	_, err := streamEndpoint.AwaitConnection(time.Second)
	require.NoError(t, err)

	context := ldcontext.New("context-key")
	flagKeyValue := basicEvaluateFlag(t, client, "flag-key", context, defaultValue)
	m.In(t).Assert(flagKeyValue, m.JSONEqual(initialValue))

	stream.streamingService.PushDelete("flag", "flag-key", 2)
	stream.streamingService.PushUpdate("flag", "new-flag-key", 1, c.makeFlagData("new-flag-key", 1, newInitialValue))

	require.Never(
		t,
		checkForUpdatedValue(t, client, "flag-key", context, initialValue, defaultValue, defaultValue),
		time.Millisecond*100,
		time.Millisecond*20,
		"flag value was updated, but it should not have been",
	)

	require.Never(
		t,
		checkForUpdatedValue(t, client, "new-flag-key", context, defaultValue, newInitialValue, defaultValue),
		time.Millisecond*100,
		time.Millisecond*20,
		"flag value was updated, but it should not have been",
	)

	stream.streamingService.PushPayloadTransferred("updated", 2)

	pollUntilFlagValueUpdated(t, client, "flag-key", context, initialValue, defaultValue, defaultValue)
	pollUntilFlagValueUpdated(t, client, "new-flag-key", context, defaultValue, newInitialValue, defaultValue)
}

func (c CommonStreamingTests) IgnoresModelVersion(t *ldtest.T) {
	data := c.makeSDKDataWithFlag("flag-key", 100, initialValue)
	stream := NewSDKDataSourceWithoutEndpoint(t, data)
	streamEndpoint := requireContext(t).harness.NewMockEndpoint(stream.Handler(), t.DebugLogger(),
		harness.MockEndpointDescription("streaming service"))
	t.Defer(streamEndpoint.Close)
	client := NewSDKClient(t, WithStreamingConfig(baseStreamConfig(streamEndpoint)))

	_, err := streamEndpoint.AwaitConnection(time.Second)
	require.NoError(t, err)

	context := ldcontext.New("context-key")
	flagKeyValue := basicEvaluateFlag(t, client, "flag-key", context, defaultValue)
	m.In(t).Assert(flagKeyValue, m.JSONEqual(initialValue))

	// This flag's version is less than the version previously given to the
	// SDK. However, the state we are sending suggests it is later. The SDK
	// should ignore the individual model version and just trust the overall
	// state version.
	stream.streamingService.PushUpdate("flag", "flag-key", 1, c.makeFlagData("flag-key", 1, updatedValue))
	stream.streamingService.PushPayloadTransferred("updated", 2)

	pollUntilFlagValueUpdated(t, client, "flag-key", context, initialValue, updatedValue, defaultValue)
}

func (c CommonStreamingTests) IgnoresHeartBeat(t *ldtest.T) {
	data := c.makeSDKDataWithFlag("flag-key", 1, initialValue)
	stream := NewSDKDataSourceWithoutEndpoint(t, data)
	streamEndpoint := requireContext(t).harness.NewMockEndpoint(stream.Handler(), t.DebugLogger(),
		harness.MockEndpointDescription("streaming service"))
	t.Defer(streamEndpoint.Close)
	client := NewSDKClient(t, WithStreamingConfig(baseStreamConfig(streamEndpoint)))

	_, err := streamEndpoint.AwaitConnection(time.Second)
	require.NoError(t, err)

	context := ldcontext.New("context-key")
	flagKeyValue := basicEvaluateFlag(t, client, "flag-key", context, defaultValue)
	m.In(t).Assert(flagKeyValue, m.JSONEqual(initialValue))

	stream.streamingService.PushHeartbeat()
	stream.streamingService.PushUpdate("flag", "flag-key", 2, c.makeFlagData("flag-key", 2, updatedValue))
	stream.streamingService.PushHeartbeat()
	stream.streamingService.PushPayloadTransferred("updated", 2)

	pollUntilFlagValueUpdated(t, client, "flag-key", context, initialValue, updatedValue, defaultValue)
}

func (c CommonStreamingTests) DiscardsEventsOnError(t *ldtest.T) {
	data := c.makeSDKDataWithFlag("flag-key", 1, initialValue)
	stream := NewSDKDataSourceWithoutEndpoint(t, data)
	streamEndpoint := requireContext(t).harness.NewMockEndpoint(stream.Handler(), t.DebugLogger(),
		harness.MockEndpointDescription("streaming service"))
	t.Defer(streamEndpoint.Close)
	client := NewSDKClient(t, WithStreamingConfig(baseStreamConfig(streamEndpoint)))

	_, err := streamEndpoint.AwaitConnection(time.Second)
	require.NoError(t, err)

	context := ldcontext.New("context-key")
	flagKeyValue := basicEvaluateFlag(t, client, "flag-key", context, defaultValue)
	m.In(t).Assert(flagKeyValue, m.JSONEqual(initialValue))

	// The error should cause this update to be discard.
	stream.streamingService.PushUpdate("flag", "flag-key", 2, c.makeFlagData("flag-key", 2, updatedValue))
	stream.streamingService.PushError("some-id", "some reason")
	// But this change should be applied.
	stream.streamingService.PushUpdate("flag", "new-flag-key", 2, c.makeFlagData("new-flag-key", 2, newInitialValue))
	stream.streamingService.PushPayloadTransferred("updated", 2)

	require.Never(
		t,
		checkForUpdatedValue(t, client, "flag-key", context, initialValue, updatedValue, defaultValue),
		time.Millisecond*100,
		time.Millisecond*20,
		"flag value was updated, but it should not have been",
	)

	pollUntilFlagValueUpdated(t, client, "new-flag-key", context, defaultValue, newInitialValue, defaultValue)
}

func (c CommonStreamingTests) DisconnectsOnGoodbye(t *ldtest.T) {
	dataBefore := c.makeSDKDataWithFlag("flag-key", 1, initialValue)
	dataAfter := mockld.NewServerSDKDataBuilder().IntentCode("xfer-none").IntentReason("up-to-date").Build()
	streamEndpoint, streams := makeSequentialStreamHandler(t, dataBefore, dataAfter)
	t.Defer(streamEndpoint.Close)
	client := NewSDKClient(t, WithStreamingConfig(baseStreamConfig(streamEndpoint)))

	_, err := streamEndpoint.AwaitConnection(time.Second)
	require.NoError(t, err)

	streams[0].streamingService.PushUpdate("flag", "flag-key", 2, c.makeFlagData("flag-key", 2, updatedValue))
	// This should prompt the SDK to discard previous events, disconnect, and then re-connect.
	streams[0].streamingService.PushGoodbye("some-reason", false, false)

	_, err = streamEndpoint.AwaitConnection(time.Second)
	require.NoError(t, err)

	context := ldcontext.New("context-key")
	require.Never(
		t,
		checkForUpdatedValue(t, client, "flag-key", context, initialValue, updatedValue, defaultValue),
		time.Millisecond*100,
		time.Millisecond*20,
		"flag value was updated, but it should not have been",
	)
}

func makeSequentialStreamHandler(t *ldtest.T, dataSources ...mockld.SDKData) (*harness.MockEndpoint, []*SDKDataSource) {
	sdkDataSources := make([]*SDKDataSource, len(dataSources))
	handlers := make([]http.Handler, len(dataSources))

	for i, data := range dataSources {
		stream := NewSDKDataSourceWithoutEndpoint(t, data)
		sdkDataSources[i] = stream
		handlers[i] = stream.Handler()
	}

	handler := httphelpers.SequentialHandler(handlers[0], handlers[1:]...)

	return requireContext(t).harness.NewMockEndpoint(handler, t.DebugLogger(),
		harness.MockEndpointDescription("streaming service")), sdkDataSources
}

func validatePayloadReceived(t *ldtest.T,
	streamEndpoint *harness.MockEndpoint, client *SDKClient,
	state string, evaluations map[string]ldvalue.Value) harness.IncomingRequestInfo {
	request, err := streamEndpoint.AwaitConnection(time.Second)
	require.NoError(t, err)

	m.In(t).Assert(request.URL.Query().Get("basis"), m.Equal(state))

	context := ldcontext.New("context-key")
	for flagKey, expectedValue := range evaluations {
		actualValue := basicEvaluateFlag(t, client, flagKey, context, defaultValue)
		m.In(t).Assert(actualValue, m.JSONEqual(expectedValue))
	}

	return request
}
