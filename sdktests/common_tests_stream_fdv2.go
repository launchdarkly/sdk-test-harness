package sdktests

import (
	"net/http"
	"time"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/stretchr/testify/require"
)

var (
	initialValue = ldvalue.String("initial value") //nolint:gochecknoglobals
	updatedValue = ldvalue.String("updated value") //nolint:gochecknoglobals
	finalValue   = ldvalue.String("final value")   //nolint:gochecknoglobals

	newInitialValue = ldvalue.String("new initial value") //nolint:gochecknoglobals

	defaultValue = ldvalue.String("default value") //nolint:gochecknoglobals

	// fdv2StreamingTestContext is the evaluation context for the CommonStreamingTests FDv2 suite: client
	// initial context and explicit EvaluateFlag calls must use the same context.
	fdv2StreamingTestContext = ldcontext.New("context-key") //nolint:gochecknoglobals
)

func (c CommonStreamingTests) FDv2(t *ldtest.T) {
	t.Run("reconnection state management", c.StateTransitions)
	t.Run(
		"updates are not complete until payload transferred is sent",
		c.UpdatesAreNotCompleteUntilPayloadTransferredIsSent)
	t.Run("handles multiple updates", c.HandlesMultipleUpdates)
	t.Run("ignores model version", c.IgnoresModelVersion)
	t.Run("ignores heart beat", c.IgnoresHeartBeat)
	t.Run("ignores unknown event", c.IgnoresUnknownEvent)
	t.Run("can discard partial events on errors", c.CanDiscardPartialEventsOnError)
	t.Run("can discard full events on errors", c.CanDiscardFullEventsOnError)
	t.Run("disconnects on goodbye", c.DisconnectsOnGoodbye)
	t.Run("recoverable fallback to secondary synchronizer", c.RecoverableFallbackToSecondarySynchronizer)
	t.Run("permanent fallback to secondary synchronizer", c.PermanentFallbackToSecondarySynchronizer)
	t.Run("recoverable fallback with recovery", c.RecoverableFallbackWithRecovery)
	t.Run("permanent fallback with recovery", c.PermanentFallbackWithRecovery)
	t.Run("fallback to FDv1 handling", c.FallbackFromFDv2ToFDv1)
}

func (c CommonStreamingTests) StateTransitions(t *ldtest.T) {
	t.Run("initializes from an empty state", c.InitializeFromEmptyState)
	t.Run("initializes from polling initializer", c.InitializeFromPollingInitializer)
	t.Run("initializes from polling initializer + streaming updates",
		c.InitializeFromPollingInitializerWithStreamingUpdates)
	t.Run("initializes from 2 polling initializers", c.InitializeFromTwoPollingInitializers)
	t.Run("saves previously known state", c.SavesPreviouslyKnownState)
	t.Run("replaces previously known state", c.ReplacesPreviouslyKnownState)
	t.Run("updates previously known state", c.UpdatesPreviouslyKnownState)
}

func (c CommonStreamingTests) InitializeFromEmptyState(t *ldtest.T) {
	dataSystem, configurers := c.setupDataSystems(t, c.makeSDKDataWithFlag(1, initialValue))
	client := c.newFDv2SDKClient(t, configurers...)

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue}
	validatePayloadReceived(t, dataSystem.Synchronizers[0].Endpoint(), client, "", expectedEvaluations)
}

func (c CommonStreamingTests) InitializeFromPollingInitializer(t *ldtest.T) {
	var dataBefore mockld.FDv2SDKData
	if c.isClientSide {
		dataBefore = c.fdv2ClientData("xfer-full", "payload-missing", "initial",
			c.makeClientSideFlag("flag-key", 1, initialValue))
	} else {
		dataBefore = c.fdv2ServerData("xfer-full", "payload-missing", "initial",
			c.makeServerSideFlag("flag-key", 1, initialValue))
	}
	var dataAfter mockld.FDv2SDKData
	if c.isClientSide {
		dataAfter = c.fdv2ClientData("none", "up-to-date", "initial")
	} else {
		dataAfter = c.fdv2ServerData("none", "up-to-date", "initial")
	}
	dataSystem := NewSDKDataSystemCustom(t, dataAfter,
		DataSystemOptionPollingInitializer(dataBefore), DataSystemOptionStreaming())
	dataSystem.CreateEndpoints()

	client := c.newFDv2SDKClient(t, dataSystem)

	_, err := dataSystem.Initializers[0].Endpoint().AwaitConnection(time.Second)
	require.NoError(t, err)

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue}
	validatePayloadReceived(t, dataSystem.Synchronizers[0].Endpoint(), client, "initial", expectedEvaluations)
}

func (c CommonStreamingTests) InitializeFromPollingInitializerWithStreamingUpdates(t *ldtest.T) {
	var dataBefore mockld.FDv2SDKData
	if c.isClientSide {
		dataBefore = c.fdv2ClientData("xfer-full", "payload-missing", "initial",
			c.makeClientSideFlag("flag-key", 1, initialValue))
	} else {
		dataBefore = c.fdv2ServerData("xfer-full", "payload-missing", "initial",
			c.makeServerSideFlag("flag-key", 1, initialValue))
	}
	var dataAfter mockld.FDv2SDKData
	if c.isClientSide {
		dataAfter = c.fdv2ClientData("xfer-changes", "stale", "initial",
			c.makeClientSideFlag("new-flag-key", 1, newInitialValue))
	} else {
		dataAfter = c.fdv2ServerData("xfer-changes", "stale", "initial",
			c.makeServerSideFlag("new-flag-key", 1, newInitialValue))
	}
	dataSystem := NewSDKDataSystemCustom(t, dataBefore,
		DataSystemOptionPollingInitializer(dataBefore), DataSystemOptionStreaming())
	dataSystem.CreateEndpoints()
	dataSystem.Synchronizers[0].streaming.SetInitialData(dataAfter)

	client := c.newFDv2SDKClient(t, dataSystem)

	_, err := dataSystem.Initializers[0].Endpoint().AwaitConnection(time.Second)
	require.NoError(t, err)

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue, "new-flag-key": newInitialValue}
	validatePayloadReceived(t, dataSystem.Synchronizers[0].Endpoint(), client, "initial", expectedEvaluations)
}

func (c CommonStreamingTests) InitializeFromTwoPollingInitializers(t *ldtest.T) {
	var emptyPayload mockld.FDv2SDKData
	if c.isClientSide {
		emptyPayload = c.fdv2ClientData("xfer-full", "payload-missing", "initial")
	} else {
		emptyPayload = c.fdv2ServerData("xfer-full", "payload-missing", "initial")
	}
	var initialStatefulData mockld.FDv2SDKData
	if c.isClientSide {
		initialStatefulData = c.fdv2ClientData("xfer-full", "payload-missing", "expected-state",
			c.makeClientSideFlag("flag-key", 2, updatedValue))
	} else {
		initialStatefulData = c.fdv2ServerData("xfer-full", "payload-missing", "expected-state",
			c.makeServerSideFlag("flag-key", 2, updatedValue))
	}
	var streamingData mockld.FDv2SDKData
	if c.isClientSide {
		streamingData = c.fdv2ClientData("none", "up-to-date", "expected-state")
	} else {
		streamingData = c.fdv2ServerData("none", "up-to-date", "expected-state")
	}
	dataSystem := NewSDKDataSystemCustom(t, streamingData,
		DataSystemOptionPollingInitializer(emptyPayload), DataSystemOptionPollingInitializer(initialStatefulData),
		DataSystemOptionStreaming())
	dataSystem.CreateEndpoints()

	// Force the first endpoint to fail
	dataSystem.Initializers[0].Endpoint().Close()

	client := c.newFDv2SDKClient(t, dataSystem)

	// Verify the initializers fall over to the next initializer in line.
	_, err := dataSystem.Initializers[1].Endpoint().AwaitConnection(time.Second)
	require.NoError(t, err)

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": updatedValue}
	validatePayloadReceived(t, dataSystem.Synchronizers[0].Endpoint(), client, "expected-state", expectedEvaluations)
}

func (c CommonStreamingTests) RecoverableFallbackToSecondarySynchronizer(t *ldtest.T) {
	t.LongRunning()

	// First synchronizer hangs (never responds) to trigger initialization timeout fallback.
	// This tests the recoverable fallback scenario where the first synchronizer is NOT removed
	// from the list (unlike non-recoverable 4xx errors), but we still fall back to the secondary
	// after failing to initialize within the timeout period (10+ seconds).
	hangingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never respond - simulates a hanging connection
		select {}
	})
	hangingEndpoint := requireContext(t).harness.NewMockEndpoint(hangingHandler, t.DebugLogger(),
		harness.MockEndpointDescription("hanging streaming service"))
	t.Defer(hangingEndpoint.Close)

	// Second synchronizer returns valid data
	streamingData := c.makeSDKDataWithFlag(1, initialValue)
	secondaryStream := mockld.NewStreamingService(streamingData, requireContext(t).sdkKind, t.DebugLogger())
	secondaryEndpoint := requireContext(t).harness.NewMockEndpoint(secondaryStream, t.DebugLogger(),
		harness.MockEndpointDescription("secondary streaming service"))
	t.Defer(secondaryEndpoint.Close)

	client := c.newFDv2SDKClient(t,
		WithConfig(servicedef.SDKConfigParams{
			// Allow initialization to eventually succeed via secondary
			StartWaitTimeMS: o.Some(ldtime.UnixMillisecondTime(30 * time.Second)),
		}),
		WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
			BaseURI: hangingEndpoint.BaseURL(),
		}),
		WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
			BaseURI: secondaryEndpoint.BaseURL(),
		}))

	// Verify the client received data from the secondary synchronizer
	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue}
	validatePayloadReceived(t, secondaryEndpoint, client, "", expectedEvaluations)
}

func (c CommonStreamingTests) PermanentFallbackToSecondarySynchronizer(t *ldtest.T) {
	if c.isClientSide {
		t.RequireCapability(servicedef.CapabilityClientEventSourceHTTPErrors)
	}

	// First synchronizer returns 401 Unauthorized, which is a non-recoverable error.
	// Non-recoverable 4xx errors (all except 400, 408, 429) cause the synchronizer to be
	// permanently removed from the list, and the SDK immediately falls back to the secondary.
	errorHandler := httphelpers.HandlerWithStatus(401)
	errorEndpoint := requireContext(t).harness.NewMockEndpoint(errorHandler, t.DebugLogger(),
		harness.MockEndpointDescription("unauthorized streaming service"))
	t.Defer(errorEndpoint.Close)

	// Second synchronizer returns valid data
	streamingData := c.makeSDKDataWithFlag(1, initialValue)
	secondaryStream := mockld.NewStreamingService(streamingData, requireContext(t).sdkKind, t.DebugLogger())
	secondaryEndpoint := requireContext(t).harness.NewMockEndpoint(secondaryStream, t.DebugLogger(),
		harness.MockEndpointDescription("secondary streaming service"))
	t.Defer(secondaryEndpoint.Close)

	client := c.newFDv2SDKClient(t,
		WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
			BaseURI: errorEndpoint.BaseURL(),
		}),
		WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
			BaseURI: secondaryEndpoint.BaseURL(),
		}))

	// Verify the client received data from the secondary synchronizer
	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue}
	validatePayloadReceived(t, secondaryEndpoint, client, "", expectedEvaluations)
}

func (c CommonStreamingTests) RecoverableFallbackWithRecovery(t *ldtest.T) {
	t.LongRunning()

	// This test verifies that after a recoverable fallback, the SDK will attempt to
	// reconnect to the original synchronizer after the 5-minute recovery period.
	//
	// Setup: 3 synchronizers
	// - First: hangs initially (triggers recoverable fallback after 10s)
	// - Second: hangs (triggers another recoverable fallback)
	// - Third: healthy, serves data
	// After 5 minutes, SDK should recover back to the first synchronizer.

	// First synchronizer: hangs on first connection, serves data on recovery
	hangingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {}
	})
	recoveryData := c.makeSDKDataWithFlag(2, updatedValue)
	recoveryStream := mockld.NewStreamingService(recoveryData, requireContext(t).sdkKind, t.DebugLogger())
	firstHandler := httphelpers.SequentialHandler(hangingHandler, recoveryStream)
	firstEndpoint := requireContext(t).harness.NewMockEndpoint(firstHandler, t.DebugLogger(),
		harness.MockEndpointDescription("first streaming service"))
	t.Defer(firstEndpoint.Close)

	// Second synchronizer hangs
	secondHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {}
	})
	secondEndpoint := requireContext(t).harness.NewMockEndpoint(secondHandler, t.DebugLogger(),
		harness.MockEndpointDescription("second streaming service"))
	t.Defer(secondEndpoint.Close)

	// Third synchronizer returns valid data
	streamingData := c.makeSDKDataWithFlag(1, initialValue)
	thirdStream := mockld.NewStreamingService(streamingData, requireContext(t).sdkKind, t.DebugLogger())
	thirdEndpoint := requireContext(t).harness.NewMockEndpoint(thirdStream, t.DebugLogger(),
		harness.MockEndpointDescription("third streaming service"))
	t.Defer(thirdEndpoint.Close)

	client := c.newFDv2SDKClient(t,
		WithConfig(servicedef.SDKConfigParams{
			StartWaitTimeMS: o.Some(ldtime.UnixMillisecondTime(60000)),
		}),
		WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
			BaseURI: firstEndpoint.BaseURL(),
		}),
		WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
			BaseURI: secondEndpoint.BaseURL(),
		}),
		WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
			BaseURI: thirdEndpoint.BaseURL(),
		}))

	// Verify initial data from third synchronizer
	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue}
	validatePayloadReceived(t, thirdEndpoint, client, "", expectedEvaluations)

	// Send periodic heartbeats to keep the third stream connection alive
	// (prevents SSE read timeout from firing before recovery condition is met)
	stopHeartbeats := make(chan struct{})
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				thirdStream.PushHeartbeat()
			case <-stopHeartbeats:
				return
			}
		}
	}()
	defer close(stopHeartbeats)

	// Wait for recovery period (5 minutes + buffer)
	time.Sleep(5*time.Minute + 15*time.Second)

	// Verify SDK recovered back to first synchronizer with updated data.
	// The sequential handler serves updatedValue only on the second connection,
	// so receiving it proves the SDK reconnected after recovery.
	expectedEvaluations = map[string]ldvalue.Value{"flag-key": updatedValue}
	validatePayloadReceived(t, firstEndpoint, client, "", expectedEvaluations)
}

func (c CommonStreamingTests) PermanentFallbackWithRecovery(t *ldtest.T) {
	t.LongRunning()

	// This test verifies that after a permanent removal (non-recoverable error), the SDK
	// will NOT attempt to reconnect to that synchronizer, but WILL recover to other
	// synchronizers that had recoverable failures.
	//
	// Setup: 3 synchronizers
	// - First: returns 401 (permanent removal, non-recoverable)
	// - Second: hangs initially (triggers recoverable fallback)
	// - Third: healthy, serves data
	// After 5 minutes, SDK should recover to second synchronizer (NOT first).

	// First synchronizer returns 401 (permanent removal)
	firstHandler := httphelpers.HandlerWithStatus(401)
	firstEndpoint := requireContext(t).harness.NewMockEndpoint(firstHandler, t.DebugLogger(),
		harness.MockEndpointDescription("unauthorized streaming service"))
	t.Defer(firstEndpoint.Close)

	// Second synchronizer: hangs on first connection, serves data on recovery
	hangingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {}
	})
	recoveryData := c.makeSDKDataWithFlag(2, updatedValue)
	recoveryStream := mockld.NewStreamingService(recoveryData, requireContext(t).sdkKind, t.DebugLogger())
	secondHandler := httphelpers.SequentialHandler(hangingHandler, recoveryStream)
	secondEndpoint := requireContext(t).harness.NewMockEndpoint(secondHandler, t.DebugLogger(),
		harness.MockEndpointDescription("second streaming service"))
	t.Defer(secondEndpoint.Close)

	// Third synchronizer returns valid data
	streamingData := c.makeSDKDataWithFlag(1, initialValue)
	thirdStream := mockld.NewStreamingService(streamingData, requireContext(t).sdkKind, t.DebugLogger())
	thirdEndpoint := requireContext(t).harness.NewMockEndpoint(thirdStream, t.DebugLogger(),
		harness.MockEndpointDescription("third streaming service"))
	t.Defer(thirdEndpoint.Close)

	client := c.newFDv2SDKClient(t,
		WithConfig(servicedef.SDKConfigParams{
			StartWaitTimeMS: o.Some(ldtime.UnixMillisecondTime(60000)),
		}),
		WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
			BaseURI: firstEndpoint.BaseURL(),
		}),
		WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
			BaseURI: secondEndpoint.BaseURL(),
		}),
		WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
			BaseURI: thirdEndpoint.BaseURL(),
		}))

	// Verify initial data from third synchronizer
	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue}
	validatePayloadReceived(t, thirdEndpoint, client, "", expectedEvaluations)

	// Send periodic heartbeats to keep the third stream connection alive
	// (prevents SSE read timeout from firing before recovery condition is met)
	stopHeartbeats := make(chan struct{})
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				thirdStream.PushHeartbeat()
			case <-stopHeartbeats:
				return
			}
		}
	}()
	defer close(stopHeartbeats)

	// Wait for recovery period (5 minutes + buffer)
	time.Sleep(5*time.Minute + 15*time.Second)

	// Verify SDK recovered to second synchronizer (NOT first) with updated data.
	// The sequential handler serves updatedValue only on the second connection,
	// so receiving it proves the SDK reconnected after recovery.
	expectedEvaluations = map[string]ldvalue.Value{"flag-key": updatedValue}
	validatePayloadReceived(t, secondEndpoint, client, "", expectedEvaluations)
}

func (c CommonStreamingTests) FallbackFromFDv2ToFDv1(t *ldtest.T) {
	if c.isClientSide {
		t.RequireCapability(servicedef.CapabilityClientEventSourceHTTPErrors)
	}

	handler, channel := httphelpers.RecordingHandler(httphelpers.HandlerWithResponse(
		403, http.Header{"X-LD-FD-Fallback": []string{"true"}}, nil))
	endpoint := requireContext(t).harness.NewMockEndpoint(handler, t.DebugLogger(),
		harness.MockEndpointDescription("streaming service"))
	t.Defer(endpoint.Close)

	_ = NewSDKClient(t,
		WithConfig(servicedef.SDKConfigParams{
			InitCanFail:     true,
			StartWaitTimeMS: o.Some(ldtime.UnixMillisecondTime(1)),
		}),
		WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
			BaseURI: endpoint.BaseURL(),
		}),
		WithPollingSynchronizer(servicedef.SDKConfigPollingParams{
			BaseURI: endpoint.BaseURL(),
		}))

	h.RequireEventually(t, func() bool {
		_, _ = endpoint.AwaitConnection(time.Second * 1)

		select {
		case resp := <-channel:
			if resp.Request.URL.Path == "/sdk/latest-all" {
				return true
			}
		default:
			// no-op
		}
		return false
	}, time.Second, time.Millisecond*10, "failed to get call to fallback endpoint")
}

func (c CommonStreamingTests) SavesPreviouslyKnownState(t *ldtest.T) {
	dataBefore := c.makeSDKDataWithFlag(1, initialValue)
	var dataAfter mockld.FDv2SDKData
	if c.isClientSide {
		dataAfter = c.fdv2ClientData("none", "up-to-date", "initial")
	} else {
		dataAfter = c.fdv2ServerData("none", "up-to-date", "initial")
	}
	streamEndpoint, _ := makeSequentialStreamHandler(t, dataBefore, dataAfter)
	t.Defer(streamEndpoint.Close)
	client := c.newFDv2SDKClient(t, WithStreamingSynchronizer(baseStreamConfig(streamEndpoint)))

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue}
	request := validatePayloadReceived(t, streamEndpoint, client, "", expectedEvaluations)
	request.Cancel() // Drop the stream and allow the SDK to reconnect

	validatePayloadReceived(t, streamEndpoint, client, "initial", expectedEvaluations)
}

func (c CommonStreamingTests) ReplacesPreviouslyKnownState(t *ldtest.T) {
	dataBefore := c.makeSDKDataWithFlag(1, initialValue)
	replacement := ldvalue.String("replacement value")
	var dataAfter mockld.FDv2SDKData
	if c.isClientSide {
		dataAfter = c.fdv2ClientData("xfer-full", "cant-catchup", "initial",
			c.makeClientSideFlag("new-flag-key", 1, replacement))
	} else {
		dataAfter = c.fdv2ServerData("xfer-full", "cant-catchup", "initial",
			c.makeServerSideFlag("new-flag-key", 1, replacement))
	}
	streamEndpoint, _ := makeSequentialStreamHandler(t, dataBefore, dataAfter)
	t.Defer(streamEndpoint.Close)
	client := c.newFDv2SDKClient(t, WithStreamingSynchronizer(baseStreamConfig(streamEndpoint)))

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue, "new-flag-key": defaultValue}
	request := validatePayloadReceived(t, streamEndpoint, client, "", expectedEvaluations)
	request.Cancel() // Drop the stream and allow the SDK to reconnect

	expectedEvaluations = map[string]ldvalue.Value{
		"flag-key":     defaultValue,
		"new-flag-key": ldvalue.String("replacement value")}
	validatePayloadReceived(t, streamEndpoint, client, "initial", expectedEvaluations)
}

func (c CommonStreamingTests) UpdatesPreviouslyKnownState(t *ldtest.T) {
	dataBefore := c.makeSDKDataWithFlag(1, initialValue)
	var dataAfter mockld.FDv2SDKData
	if c.isClientSide {
		dataAfter = c.fdv2ClientData("xfer-changes", "stale", "initial",
			c.makeClientSideFlag("flag-key", 2, updatedValue),
			c.makeClientSideFlag("new-flag-key", 1, newInitialValue))
	} else {
		dataAfter = c.fdv2ServerData("xfer-changes", "stale", "initial",
			c.makeServerSideFlag("flag-key", 2, updatedValue),
			c.makeServerSideFlag("new-flag-key", 1, newInitialValue))
	}
	streamEndpoint, _ := makeSequentialStreamHandler(t, dataBefore, dataAfter)
	t.Defer(streamEndpoint.Close)
	client := c.newFDv2SDKClient(t, WithStreamingSynchronizer(baseStreamConfig(streamEndpoint)))

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue, "new-flag-key": defaultValue}
	request := validatePayloadReceived(t, streamEndpoint, client, "", expectedEvaluations)
	request.Cancel() // Drop the stream and allow the SDK to reconnect

	expectedEvaluations = map[string]ldvalue.Value{"flag-key": updatedValue, "new-flag-key": newInitialValue}
	validatePayloadReceived(t, streamEndpoint, client, "initial", expectedEvaluations)
}

func (c CommonStreamingTests) UpdatesAreNotCompleteUntilPayloadTransferredIsSent(t *ldtest.T) {
	dataSystem, configurers := c.setupDataSystems(t, c.makeSDKDataWithFlag(1, initialValue))
	client := c.newFDv2SDKClient(t, configurers...)

	_, err := dataSystem.Synchronizers[0].endpoint.AwaitConnection(time.Second)
	require.NoError(t, err)

	flagKeyValue := basicEvaluateFlag(t, client, "flag-key", fdv2StreamingTestContext, defaultValue)
	m.In(t).Assert(flagKeyValue, m.JSONEqual(initialValue))

	dataSystem.Synchronizers[0].streaming.PushDelete("flag", "flag-key", 2)
	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", "new-flag-key", 1, c.makeFlagData("new-flag-key", 1, newInitialValue))

	h.RequireNever(
		t,
		checkForUpdatedValue(t, client, "flag-key", fdv2StreamingTestContext, initialValue, defaultValue, defaultValue),
		time.Millisecond*100,
		time.Millisecond*20,
		"flag value was updated, but it should not have been",
	)

	h.RequireNever(
		t,
		checkForUpdatedValue(t, client, "new-flag-key", fdv2StreamingTestContext, defaultValue, newInitialValue, defaultValue),
		time.Millisecond*100,
		time.Millisecond*20,
		"flag value was updated, but it should not have been",
	)

	dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 2)

	pollUntilFlagValueUpdated(t, client, "flag-key", fdv2StreamingTestContext, initialValue, defaultValue, defaultValue)
	pollUntilFlagValueUpdated(t, client, "new-flag-key", fdv2StreamingTestContext, defaultValue, newInitialValue, defaultValue)
}

func (c CommonStreamingTests) HandlesMultipleUpdates(t *ldtest.T) {
	dataSystem, configurers := c.setupDataSystems(t, c.makeSDKDataWithFlag(1, initialValue))
	client := c.newFDv2SDKClient(t, configurers...)

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue}
	validatePayloadReceived(t, dataSystem.Synchronizers[0].Endpoint(), client, "", expectedEvaluations)

	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", "flag-key", 2, c.makeFlagData("flag-key", 2, updatedValue))
	dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 2)
	pollUntilFlagValueUpdated(t, client, "flag-key", fdv2StreamingTestContext, initialValue, updatedValue, defaultValue)

	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", "new-flag-key", 2, c.makeFlagData("new-flag-key", 2, newInitialValue))
	dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 3)
	pollUntilFlagValueUpdated(t, client, "new-flag-key", fdv2StreamingTestContext, defaultValue, newInitialValue, defaultValue)

	dataSystem.Synchronizers[0].streaming.PushServerIntent("xfer-full", "stale")
	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", "flag-key", 3, c.makeFlagData("flag-key", 3, finalValue))
	dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 4)
	pollUntilFlagValueUpdated(t, client, "flag-key", fdv2StreamingTestContext, updatedValue, finalValue, defaultValue)
	pollUntilFlagValueUpdated(t, client, "new-flag-key", fdv2StreamingTestContext, newInitialValue, defaultValue, defaultValue)
}

func (c CommonStreamingTests) IgnoresModelVersion(t *ldtest.T) {
	dataSystem, configurers := c.setupDataSystems(t, c.makeSDKDataWithFlag(100, initialValue))
	client := c.newFDv2SDKClient(t, configurers...)

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue}
	validatePayloadReceived(t, dataSystem.Synchronizers[0].Endpoint(), client, "", expectedEvaluations)

	// This flag's version is less than the version previously given to the
	// SDK. However, the state we are sending suggests it is later. The SDK
	// should ignore the individual model version and just trust the overall
	// state version.
	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", "flag-key", 1, c.makeFlagData("flag-key", 1, updatedValue))
	dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 2)

	pollUntilFlagValueUpdated(t, client, "flag-key", fdv2StreamingTestContext, initialValue, updatedValue, defaultValue)
}

func (c CommonStreamingTests) IgnoresHeartBeat(t *ldtest.T) {
	dataSystem, configurers := c.setupDataSystems(t, c.makeSDKDataWithFlag(1, initialValue))
	client := c.newFDv2SDKClient(t, configurers...)

	_, err := dataSystem.Synchronizers[0].endpoint.AwaitConnection(time.Second)
	require.NoError(t, err)

	flagKeyValue := basicEvaluateFlag(t, client, "flag-key", fdv2StreamingTestContext, defaultValue)
	m.In(t).Assert(flagKeyValue, m.JSONEqual(initialValue))

	dataSystem.Synchronizers[0].streaming.PushHeartbeat()
	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", "flag-key", 2, c.makeFlagData("flag-key", 2, updatedValue))
	dataSystem.Synchronizers[0].streaming.PushHeartbeat()
	dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 2)

	pollUntilFlagValueUpdated(t, client, "flag-key", fdv2StreamingTestContext, initialValue, updatedValue, defaultValue)
}

func (c CommonStreamingTests) IgnoresUnknownEvent(t *ldtest.T) {
	t.Run("unknown event in the middle of a payload", c.IgnoresUnknownEventMiddleOfPayload)
	t.Run("unknown event at the start of a payload", c.IgnoresUnknownEventStartOfPayload)
}

func (c CommonStreamingTests) IgnoresUnknownEventMiddleOfPayload(t *ldtest.T) {
	dataSystem, configurers := c.setupDataSystems(t, c.makeSDKDataWithFlag(1, initialValue))
	client := c.newFDv2SDKClient(t, configurers...)

	_, err := dataSystem.Synchronizers[0].endpoint.AwaitConnection(time.Second)
	require.NoError(t, err)

	flagKeyValue := basicEvaluateFlag(t, client, "flag-key", fdv2StreamingTestContext, defaultValue)
	m.In(t).Assert(flagKeyValue, m.JSONEqual(initialValue))

	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", "flag-key", 2, c.makeFlagData("flag-key", 2, updatedValue))
	dataSystem.Synchronizers[0].streaming.PushEvent("unknown-event-type", map[string]interface{}{
		"some": "data",
	})
	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", "new-flag-key", 1, c.makeFlagData("new-flag-key", 1, newInitialValue))
	dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 2)

	pollUntilFlagValueUpdated(t, client, "flag-key", fdv2StreamingTestContext, initialValue, updatedValue, defaultValue)
	pollUntilFlagValueUpdated(t, client, "new-flag-key", fdv2StreamingTestContext, defaultValue, newInitialValue, defaultValue)
}

func (c CommonStreamingTests) IgnoresUnknownEventStartOfPayload(t *ldtest.T) {
	dataSystem, configurers := c.setupDataSystems(t, c.makeSDKDataWithFlag(1, initialValue))
	client := c.newFDv2SDKClient(t, configurers...)

	_, err := dataSystem.Synchronizers[0].endpoint.AwaitConnection(time.Second)
	require.NoError(t, err)

	flagKeyValue := basicEvaluateFlag(t, client, "flag-key", fdv2StreamingTestContext, defaultValue)
	m.In(t).Assert(flagKeyValue, m.JSONEqual(initialValue))

	dataSystem.Synchronizers[0].streaming.PushEvent("unknown-event-type", map[string]interface{}{
		"some": "data",
	})
	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", "flag-key", 2, c.makeFlagData("flag-key", 2, updatedValue))
	dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 2)

	pollUntilFlagValueUpdated(t, client, "flag-key", fdv2StreamingTestContext, initialValue, updatedValue, defaultValue)
}

func (c CommonStreamingTests) CanDiscardPartialEventsOnError(t *ldtest.T) {
	dataSystem, configurers := c.setupDataSystems(t, c.makeSDKDataWithFlag(1, initialValue))
	client := c.newFDv2SDKClient(t, configurers...)

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue}
	validatePayloadReceived(t, dataSystem.Synchronizers[0].Endpoint(), client, "", expectedEvaluations)

	// The error should cause this update to be discard.
	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", "flag-key", 2, c.makeFlagData("flag-key", 2, updatedValue))
	dataSystem.Synchronizers[0].streaming.PushError("some-id", "some reason")
	// But this change should be applied.
	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", "new-flag-key", 2, c.makeFlagData("new-flag-key", 2, newInitialValue))
	dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 2)

	h.RequireNever(
		t,
		checkForUpdatedValue(t, client, "flag-key", fdv2StreamingTestContext, initialValue, updatedValue, defaultValue),
		time.Millisecond*100,
		time.Millisecond*20,
		"flag value was updated, but it should not have been",
	)

	pollUntilFlagValueUpdated(t, client, "new-flag-key", fdv2StreamingTestContext, defaultValue, newInitialValue, defaultValue)

	// Original flag value should still be the same.
	value := basicEvaluateFlag(t, client, "flag-key", fdv2StreamingTestContext, defaultValue)
	require.Equal(t, initialValue, value)
}

func (c CommonStreamingTests) CanDiscardFullEventsOnError(t *ldtest.T) {
	dataSystem, configurers := c.setupDataSystems(t, c.makeSDKDataWithFlag(1, initialValue))
	client := c.newFDv2SDKClient(t, configurers...)

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue}
	validatePayloadReceived(t, dataSystem.Synchronizers[0].Endpoint(), client, "", expectedEvaluations)

	// The error should cause this update to be discard.
	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", "flag-key", 2, c.makeFlagData("flag-key", 2, updatedValue))
	dataSystem.Synchronizers[0].streaming.PushError("some-id", "some reason")
	// But this change should be applied.
	dataSystem.Synchronizers[0].streaming.PushServerIntent("xfer-full", "stale")
	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", "new-flag-key", 2, c.makeFlagData("new-flag-key", 2, newInitialValue))
	dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 2)

	// Previous flag should be removed, reverting to a default value being served.
	pollUntilFlagValueUpdated(t, client, "flag-key", fdv2StreamingTestContext, initialValue, defaultValue, defaultValue)
	pollUntilFlagValueUpdated(t, client, "new-flag-key", fdv2StreamingTestContext, defaultValue, newInitialValue, defaultValue)
}

func (c CommonStreamingTests) DisconnectsOnGoodbye(t *ldtest.T) {
	dataBefore := c.makeSDKDataWithFlag(1, initialValue)
	var dataAfter mockld.FDv2SDKData
	if c.isClientSide {
		dataAfter = c.fdv2ClientData("none", "up-to-date", "initial")
	} else {
		dataAfter = c.fdv2ServerData("none", "up-to-date", "initial")
	}
	streamEndpoint, dataSystems := makeSequentialStreamHandler(t, dataBefore, dataAfter)
	t.Defer(streamEndpoint.Close)
	client := c.newFDv2SDKClient(t, WithStreamingSynchronizer(baseStreamConfig(streamEndpoint)))

	conn := streamEndpoint.RequireConnection(t, time.Second)

	dataSystems[0].Synchronizers[0].streaming.PushUpdate(
		"flag", "flag-key", 2, c.makeFlagData("flag-key", 2, updatedValue))
	// This should prompt the SDK to discard previous events, disconnect, and then re-connect.
	dataSystems[0].Synchronizers[0].streaming.PushGoodbye("some-reason", false, false)
	conn.Cancel()

	_ = streamEndpoint.RequireConnection(t, time.Second)

	h.RequireNever(
		t,
		checkForUpdatedValue(t, client, "flag-key", fdv2StreamingTestContext, initialValue, updatedValue, defaultValue),
		time.Millisecond*100,
		time.Millisecond*20,
		"flag value was updated, but it should not have been",
	)
}

func makeSequentialStreamHandler(t *ldtest.T, dataSources ...mockld.SDKData) (
	*harness.MockEndpoint, []*SDKDataSystem) {
	handlers := make([]http.Handler, len(dataSources))
	dataSystems := make([]*SDKDataSystem, len(dataSources))

	for i, data := range dataSources {
		dataSystem := NewSDKDataSystem(t, data)
		handlers[i] = dataSystem.Synchronizers[0].streaming
		dataSystems[i] = dataSystem
	}

	handler := httphelpers.SequentialHandler(handlers[0], handlers[1:]...)

	return requireContext(t).harness.NewMockEndpoint(handler, t.DebugLogger(),
		harness.MockEndpointDescription("streaming service")), dataSystems
}

func validatePayloadReceived(t *ldtest.T,
	streamEndpoint *harness.MockEndpoint, client *SDKClient,
	state string, evaluations map[string]ldvalue.Value) harness.IncomingRequestInfo {
	request, err := streamEndpoint.AwaitConnection(time.Second)
	require.NoError(t, err)

	m.In(t).Assert(request.URL.Query().Get("basis"), m.Equal(state))

	h.RequireEventually(t, func() bool {
		for flagKey, expectedValue := range evaluations {
			actualValue := basicEvaluateFlag(t, client, flagKey, fdv2StreamingTestContext, defaultValue)
			if !m.In(t).Assert(actualValue, m.JSONEqual(expectedValue)) {
				return false
			}
		}

		return true
	}, time.Second, time.Millisecond*20, "failed to evaluate flag")

	return request
}
