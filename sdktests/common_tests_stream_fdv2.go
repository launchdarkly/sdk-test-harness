package sdktests

import (
	"encoding/json"
	"fmt"
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

// serverSideFDv1AllData builds an FDv1-format polling response body containing a single flag.
// The FDv1 wire format is a single JSON object with `"flags"` and `"segments"` keys, distinct
// from the FDv2 polling wire format. Tests use this to stand up a realistic FDv1 Fallback
// Synchronizer endpoint.
func serverSideFDv1AllData(
	t *ldtest.T, c CommonStreamingTests, flagKey string,
	version int,
	value ldvalue.Value,
) []byte {
	t.Helper()
	body := map[string]any{
		"flags": map[string]json.RawMessage{
			flagKey: c.makeFlagData(flagKey, version, value),
		},
		"segments": map[string]json.RawMessage{},
	}
	bytes, err := json.Marshal(body)
	if err != nil {
		panic(fmt.Errorf("failed to marshal FDv1 polling body: %w", err))
	}
	return bytes
}

var (
	initialValue = ldvalue.String("initial value") //nolint:gochecknoglobals
	updatedValue = ldvalue.String("updated value") //nolint:gochecknoglobals
	finalValue   = ldvalue.String("final value")   //nolint:gochecknoglobals

	newInitialValue = ldvalue.String("new initial value") //nolint:gochecknoglobals

	defaultValue = ldvalue.String("default value") //nolint:gochecknoglobals
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
	t.Run("FDv1 fallback directive", c.FDv1FallbackDirective)
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

// DATASYSTEM 1.1.1: SDK initializes from a single synchronizer with a full basis
func (c CommonStreamingTests) InitializeFromEmptyState(t *ldtest.T) {
	dataSystem, configurers := c.setupDataSystems(t, c.makeSDKDataWithFlag(1, initialValue))
	client := NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue}
	validatePayloadReceived(t, dataSystem.Synchronizers[0].endpoint, client, "", expectedEvaluations)
}

// DATASYSTEM 1.1.2: initializers run in order; synchronizer receives basis state
func (c CommonStreamingTests) InitializeFromPollingInitializer(t *ldtest.T) {
	dataBefore := mockld.NewServerSDKDataBuilder().Flag(c.makeServerSideFlag("flag-key", 1, initialValue)).Build()
	dataAfter := mockld.NewServerSDKDataBuilder().IntentCode("none").IntentReason("up-to-date").Build()
	dataSystem := NewSDKDataSystem(t, dataAfter, DataSystemOptionPollingInitializer(dataBefore))

	client := NewSDKClient(t, dataSystem)

	_, err := dataSystem.Initializers[0].Endpoint().AwaitConnection(time.Second)
	require.NoError(t, err)

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue}
	validatePayloadReceived(t, dataSystem.Synchronizers[0].Endpoint(), client, "initial", expectedEvaluations)
}

// DATASYSTEM 1.1.5: initializer basis + synchronizer xfer-changes merged at startup
func (c CommonStreamingTests) InitializeFromPollingInitializerWithStreamingUpdates(t *ldtest.T) {
	dataBefore := mockld.NewServerSDKDataBuilder().
		Flag(c.makeServerSideFlag("flag-key", 1, initialValue)).
		Build()
	dataAfter := mockld.NewServerSDKDataBuilder().
		IntentCode("xfer-changes").
		IntentReason("stale").
		Flag(c.makeServerSideFlag("new-flag-key", 1, newInitialValue)).
		Build()
	dataSystem := NewSDKDataSystem(t, dataBefore, DataSystemOptionPollingInitializer(dataBefore))
	dataSystem.Synchronizers[0].streaming.SetInitialData(dataAfter)

	client := NewSDKClient(t, dataSystem)

	_, err := dataSystem.Initializers[0].Endpoint().AwaitConnection(time.Second)
	require.NoError(t, err)

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue, "new-flag-key": newInitialValue}
	validatePayloadReceived(t, dataSystem.Synchronizers[0].Endpoint(), client, "initial", expectedEvaluations)
}

// DATASYSTEM 1.1.2, 1.1.3: first initializer failure falls through to next initializer
func (c CommonStreamingTests) InitializeFromTwoPollingInitializers(t *ldtest.T) {
	emptyPayload := mockld.NewServerSDKDataBuilder().
		Build()
	initialStatefulData := mockld.NewServerSDKDataBuilder().
		IntentCode("xfer-full").
		IntentReason("payload-missing").
		State("expected-state").
		Flag(c.makeServerSideFlag("flag-key", 2, updatedValue)).
		Build()
	streamingData := mockld.NewServerSDKDataBuilder().
		IntentCode("none").
		IntentReason("up-to-date").
		State("expected-state").
		Build()
	dataSystem := NewSDKDataSystem(t, streamingData,
		DataSystemOptionPollingInitializer(emptyPayload), DataSystemOptionPollingInitializer(initialStatefulData))

	// Force the first endpoint to fail
	dataSystem.Initializers[0].Endpoint().Close()

	client := NewSDKClient(t, dataSystem)

	// Verify the initializers fall over to the next initializer in line.
	_, err := dataSystem.Initializers[1].Endpoint().AwaitConnection(time.Second)
	require.NoError(t, err)

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": updatedValue}
	validatePayloadReceived(t, dataSystem.Synchronizers[0].Endpoint(), client, "expected-state", expectedEvaluations)
}

// DATASYSTEM 1.2.7: recoverable error triggers fallback to next synchronizer
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

	client := NewSDKClient(t,
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

// DATASYSTEM 1.2.7: non-recoverable 4xx error permanently removes synchronizer, falls back immediately
func (c CommonStreamingTests) PermanentFallbackToSecondarySynchronizer(t *ldtest.T) {
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

	client := NewSDKClient(t,
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

// DATASYSTEM 1.2.8: after recovery period, SDK attempts reconnection to earlier synchronizers
// CSFDV2 8.2.1: client-side recovery timeout default is 300 seconds (VALID condition)
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

	client := NewSDKClient(t,
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

// DATASYSTEM 1.2.8: permanently removed synchronizers excluded from recovery; recoverable ones revisited
// CSFDV2 8.2.1: client-side recovery timeout default is 300 seconds (VALID condition)
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

	client := NewSDKClient(t,
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

// fdv1PollingPath is the FDv1 polling URL path. When the FDv1 Fallback Synchronizer
// is engaged, the SDK must request this path — not the FDv2 /sdk/poll path — so tests
// can distinguish between an FDv2-level fallback and a directed FDv1 fallback by URL
// inspection alone.
const fdv1PollingPath = "/sdk/latest-all"

// FDv1FallbackDirective groups the server-directed FDv1 fallback tests defined by
// section 1.6 of the Data System spec. See:
// specs/DATASYSTEM-data-system/v2/README.md#16-fdv1-fallback-directive
//
// These tests are gated on the "fdv1-fallback" capability so SDKs that have not yet
// implemented the directive can opt out. Once the behavior is ubiquitous across SDKs,
// dropping the capability from every test service decommissions the suite cleanly.
func (c CommonStreamingTests) FDv1FallbackDirective(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityFDv1Fallback)

	t.Run("directive on streaming error engages FDv1 fallback",
		c.DirectiveOnStreamingErrorEngagesFDv1)
	t.Run("directive on streaming success applies payload then engages FDv1",
		c.DirectiveOnStreamingSuccessAppliesPayload)
	t.Run("directive on polling initializer skips FDv2 synchronizers",
		c.DirectiveOnPollingInitializerSkipsSynchronizers)
	t.Run("directive without FDv1 fallback configured halts the data system",
		c.DirectiveWithoutFDv1ConfiguredHaltsDataSystem)
	t.Run("directed fallback is terminal and does not revisit FDv2 sources",
		c.DirectedFallbackIsTerminal)
}

// DirectiveOnStreamingErrorEngagesFDv1 verifies Requirement 1.6.1 and 1.6.3 for the
// error path: an `X-LD-FD-Fallback: true` header accompanying an error response on the
// streaming synchronizer MUST engage the FDv1 Fallback Synchronizer. The directive is
// honored regardless of status code.
//
// The error response carries no payload, so the SDK will have no data until FDv1
// serves it. The FDv1 endpoint therefore delivers a distinctive flag value, and the
// test asserts evaluations return that value — proving FDv1 actually became the
// active data source, not just that its URL was hit.
//
// DATASYSTEM 1.6.1: X-LD-FD-Fallback header on error engages FDv1 Fallback Synchronizer
// DATASYSTEM 1.6.3: FDv2 synchronizer chain halted when directive engages FDv1
// CSFDV2 8.1.1: x-ld-fd-fallback header triggers client-side FDv1 fallback
// CSFDV2 8.1.3: FDv2 synchronizers disabled, not removed
func (c CommonStreamingTests) DirectiveOnStreamingErrorEngagesFDv1(t *ldtest.T) {
	streamHandler, _ := httphelpers.RecordingHandler(httphelpers.HandlerWithResponse(
		403, http.Header{"X-LD-FD-Fallback": []string{"true"}}, nil))
	streamEndpoint := requireContext(t).harness.NewMockEndpoint(streamHandler, t.DebugLogger(),
		harness.MockEndpointDescription("FDv2 streaming service (403 + directive)"))
	t.Defer(streamEndpoint.Close)

	fdv1Value := ldvalue.String("value-from-fdv1")
	fdv1Handler, fdv1Channel := httphelpers.RecordingHandler(
		httphelpers.HandlerWithResponse(200, http.Header{"Content-Type": []string{"application/json"}},
			serverSideFDv1AllData(t, c, "flag-key", 1, fdv1Value)))
	fdv1Endpoint := requireContext(t).harness.NewMockEndpoint(fdv1Handler, t.DebugLogger(),
		harness.MockEndpointDescription("FDv1 polling service"))
	t.Defer(fdv1Endpoint.Close)

	client := NewSDKClient(t,
		WithConfig(servicedef.SDKConfigParams{
			StartWaitTimeMS: o.Some(ldtime.UnixMillisecondTime(5 * time.Second / time.Millisecond)),
		}),
		WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
			BaseURI: streamEndpoint.BaseURL(),
		}),
		WithFDv1Fallback(servicedef.SDKConfigPollingParams{
			BaseURI: fdv1Endpoint.BaseURL(),
		}))

	// Drain the initial FDv2 connection so the quiescence check below has a
	// stable baseline.
	_, err := streamEndpoint.AwaitConnection(time.Second)
	require.NoError(t, err)

	// FDv1 endpoint must be contacted at /sdk/latest-all.
	h.RequireEventually(t, func() bool {
		select {
		case resp := <-fdv1Channel:
			return resp.Request.URL.Path == fdv1PollingPath
		default:
			return false
		}
	}, time.Second*3, time.Millisecond*10,
		"FDv1 fallback endpoint was never contacted after directive on streaming error")

	// Evaluation must reflect FDv1's payload. If we saw the default here, the SDK
	// routed to FDv1 but never finished wiring FDv1's data into the Memory Store.
	context := ldcontext.New("context-key")
	h.RequireEventually(t, func() bool {
		value := basicEvaluateFlag(t, client, "flag-key", context, defaultValue)
		return m.In(t).Assert(value, m.JSONEqual(fdv1Value))
	}, time.Second*3, time.Millisecond*20,
		"flag-key should have been served from the FDv1 fallback after directive on streaming error")

	// FDv2 streaming must be quiet — no concurrent retries against the (now
	// halted) Primary Synchronizer. Per 1.6.3(2), the FDv2 chain must be stopped
	// once the directive engages the FDv1 fallback, so observing a second
	// connection here would mean the SDK is running both data sources in
	// parallel. 403 alone would already have caused permanent removal, so this
	// check is defensive — it catches the case where the SDK permanently removes
	// one synchronizer but still has a background loop dialing the endpoint.
	streamEndpoint.RequireNoMoreConnections(t, time.Millisecond*500)
}

// DirectiveOnStreamingSuccessAppliesPayload verifies Requirement 1.6.2: when the
// directive arrives alongside a valid payload (here, a 200 SSE handshake that delivers
// a full basis), the SDK MUST apply that payload to the Memory Store before surfacing
// the directive and transitioning to the FDv1 Fallback Synchronizer.
//
// The streaming endpoint serves a distinctive value (streamingValue). The FDv1
// fallback endpoint responds with HTTP 400 — a transport-level error that delivers
// no body the SDK can install as a Basis. The RecordingHandler still publishes the
// request to the channel so the test can assert the directive was honored, but
// because no payload arrives via FDv1, any flag value observed in evaluations can
// only have come from the streaming payload: seeing streamingValue proves 1.6.2
// was honored, while seeing defaultValue proves the payload was dropped before
// the transition.
//
// 400 is chosen because it is a protocol-valid response that the SDK must handle
// uniformly across runtimes. An earlier iteration of this test used an
// unconditional HTTP 304, but 304 is only valid in response to a conditional
// request whose validators match (RFC 9111 §4.3.4); browser-based runtimes in
// particular translate an unsolicited 304 into a 200 against their cache, which
// would defeat the "FDv1 supplies no fresh data" property this test relies on.
//
// DATASYSTEM 1.6.2: payload applied to Memory Store before transitioning to FDv1
// DATASYSTEM 1.6.3: FDv2 Primary Synchronizer stopped when directive engages FDv1
// CSFDV2 8.1.1: x-ld-fd-fallback header triggers client-side FDv1 fallback
// CSFDV2 8.1.3: FDv2 synchronizers disabled, not removed
func (c CommonStreamingTests) DirectiveOnStreamingSuccessAppliesPayload(t *ldtest.T) {
	streamingValue := ldvalue.String("value-from-streaming-payload")
	streamingData := c.makeSDKDataWithFlag(1, streamingValue)
	streamingService := mockld.NewStreamingService(streamingData, requireContext(t).sdkKind, t.DebugLogger())
	streamingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-LD-FD-Fallback", "true")
		streamingService.ServeHTTP(w, r)
	})
	streamingEndpoint := requireContext(t).harness.NewMockEndpoint(streamingHandler, t.DebugLogger(),
		harness.MockEndpointDescription("streaming service with FDv1 directive"))
	t.Defer(streamingEndpoint.Close)

	// FDv1 fallback endpoint: returns HTTP 400 on every request. The directive has
	// already steered the SDK here (which is what we want to verify), but the 400
	// response delivers no payload the SDK can install — leaving the streaming
	// basis as the sole source of flag data in the Memory Store.
	fdv1Handler, fdv1Channel := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(400))
	fdv1Endpoint := requireContext(t).harness.NewMockEndpoint(fdv1Handler, t.DebugLogger(),
		harness.MockEndpointDescription("FDv1 polling service (HTTP 400)"))
	t.Defer(fdv1Endpoint.Close)

	client := NewSDKClient(t,
		WithConfig(servicedef.SDKConfigParams{
			StartWaitTimeMS: o.Some(ldtime.UnixMillisecondTime(5 * time.Second / time.Millisecond)),
		}),
		WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
			BaseURI: streamingEndpoint.BaseURL(),
		}),
		WithFDv1Fallback(servicedef.SDKConfigPollingParams{
			BaseURI: fdv1Endpoint.BaseURL(),
		}))

	// The streaming handshake must happen before the directive can take effect.
	streamingRequest, err := streamingEndpoint.AwaitConnection(time.Second)
	require.NoError(t, err)

	// The directive must drive the SDK to the FDv1 polling path.
	h.RequireEventually(t, func() bool {
		select {
		case resp := <-fdv1Channel:
			return resp.Request.URL.Path == fdv1PollingPath
		default:
			return false
		}
	}, time.Second*3, time.Millisecond*10,
		"FDv1 fallback endpoint was never contacted after successful directive")

	// The streaming payload must be the active data. FDv1 returns 400 and so
	// delivers no payload of its own, so seeing streamingValue here proves the SDK
	// applied the streaming basis before transitioning (1.6.2). Seeing
	// defaultValue would mean the SDK dropped the payload somewhere along the
	// handoff.
	context := ldcontext.New("context-key")
	h.RequireEventually(t, func() bool {
		value := basicEvaluateFlag(t, client, "flag-key", context, defaultValue)
		return m.In(t).Assert(value, m.JSONEqual(streamingValue))
	}, time.Second*3, time.Millisecond*20,
		"flag-key should have been served from the streaming payload after the directive was honored — "+
			"seeing the default value indicates the payload was dropped rather than applied per 1.6.2")

	// The SDK must also have Stopped the FDv2 Primary Synchronizer (1.6.3(2)) when
	// it transitioned. The request context on the streaming connection is tied to
	// the underlying TCP connection; if the SDK correctly closed the stream, the
	// context will have been cancelled. Observing an open stream here would mean
	// the FDv2 data source is still running concurrently with the FDv1 fallback,
	// which the spec forbids. We also assert no reconnection attempts arrive on
	// the streaming endpoint — if the SDK closed the stream but kept the
	// synchronizer running, it would try to reopen.
	h.RequireEventually(t, func() bool {
		select {
		case <-streamingRequest.Context.Done():
			return true
		default:
			return false
		}
	}, time.Second*3, time.Millisecond*20,
		"SDK did not close the FDv2 streaming connection after the directive — "+
			"the Primary Synchronizer must be stopped when Directed Fallback engages")
	streamingEndpoint.RequireNoMoreConnections(t, time.Millisecond*500)
}

// DirectiveOnPollingInitializerSkipsSynchronizers verifies Requirement 1.6.3(2):
// when an Initializer surfaces the FDv1 directive, the SDK MUST halt the ordinary
// initializer-then-synchronizer flow. In particular, the configured FDv2 Primary
// Synchronizer must never be started; the SDK transitions directly to the FDv1
// Fallback Synchronizer.
//
// DATASYSTEM 1.6.3: initializer directive skips FDv2 synchronizers entirely
// CSFDV2 8.1.1: x-ld-fd-fallback header triggers client-side FDv1 fallback
// CSFDV2 8.1.3: FDv2 synchronizers never started after directive
func (c CommonStreamingTests) DirectiveOnPollingInitializerSkipsSynchronizers(t *ldtest.T) {
	// Initializer endpoint: 500 + directive. No payload accompanies the directive in
	// this variant, so there is nothing to apply beforehand (1.6.2 is a no-op here).
	initHandler, _ := httphelpers.RecordingHandler(httphelpers.HandlerWithResponse(
		500, http.Header{"X-LD-FD-Fallback": []string{"true"}}, nil))
	initEndpoint := requireContext(t).harness.NewMockEndpoint(initHandler, t.DebugLogger(),
		harness.MockEndpointDescription("polling initializer"))
	t.Defer(initEndpoint.Close)

	// Streaming synchronizer endpoint: if the SDK ever contacts this, the directive
	// was not honored — per 1.6.3, no further FDv2 synchronizer may be started.
	streamHandler, streamChannel := httphelpers.RecordingHandler(
		httphelpers.HandlerWithStatus(500))
	streamEndpoint := requireContext(t).harness.NewMockEndpoint(streamHandler, t.DebugLogger(),
		harness.MockEndpointDescription("streaming synchronizer (must not be contacted)"))
	t.Defer(streamEndpoint.Close)

	// FDv1 polling endpoint: receives traffic via /sdk/latest-all once the directive
	// takes effect, and serves a distinctive flag value so we can tell the data
	// actually flowed through the FDv1 path into the Memory Store.
	fdv1Value := ldvalue.String("value-from-fdv1")
	fdv1Handler, fdv1Channel := httphelpers.RecordingHandler(
		httphelpers.HandlerWithResponse(200, http.Header{"Content-Type": []string{"application/json"}},
			serverSideFDv1AllData(t, c, "flag-key", 1, fdv1Value)))
	fdv1Endpoint := requireContext(t).harness.NewMockEndpoint(fdv1Handler, t.DebugLogger(),
		harness.MockEndpointDescription("FDv1 polling service"))
	t.Defer(fdv1Endpoint.Close)

	client := NewSDKClient(t,
		WithConfig(servicedef.SDKConfigParams{
			StartWaitTimeMS: o.Some(ldtime.UnixMillisecondTime(5 * time.Second / time.Millisecond)),
			DataSystem: o.Some(servicedef.DataSystem{
				Initializers: []servicedef.DataInitializer{
					{
						Polling: o.Some(servicedef.SDKConfigPollingParams{
							BaseURI: initEndpoint.BaseURL(),
						}),
					},
				},
				Synchronizers: []servicedef.DataSynchronizer{
					{
						Streaming: o.Some(servicedef.SDKConfigStreamingParams{
							BaseURI: streamEndpoint.BaseURL(),
						}),
					},
				},
				FDv1Fallback: o.Some(servicedef.SDKConfigPollingParams{
					BaseURI: fdv1Endpoint.BaseURL(),
				}),
			}),
		}))

	// The FDv1 fallback endpoint must see traffic at /sdk/latest-all.
	h.RequireEventually(t, func() bool {
		select {
		case resp := <-fdv1Channel:
			return resp.Request.URL.Path == fdv1PollingPath
		default:
			return false
		}
	}, time.Second*3, time.Millisecond*10, "FDv1 fallback endpoint was never contacted after init directive")

	// The FDv2 streaming synchronizer must never have been contacted. Any entry in
	// streamChannel means the SDK started the streaming synchronizer despite the
	// directive, which violates 1.6.3(2).
	h.RequireNever(t, func() bool {
		select {
		case <-streamChannel:
			return true
		default:
			return false
		}
	}, time.Millisecond*500, time.Millisecond*10,
		"FDv2 streaming synchronizer was contacted after initializer directive was received")

	// Evaluation must return FDv1's value. Simply having the FDv1 endpoint hit is
	// not enough — the directive path must also install FDv1's payload as the active
	// data, or evaluations would fall through to the default.
	context := ldcontext.New("context-key")
	h.RequireEventually(t, func() bool {
		value := basicEvaluateFlag(t, client, "flag-key", context, defaultValue)
		return m.In(t).Assert(value, m.JSONEqual(fdv1Value))
	}, time.Second*3, time.Millisecond*20,
		"flag-key should have been served from the FDv1 fallback after initializer directive")
}

// DirectiveWithoutFDv1ConfiguredHaltsDataSystem verifies Requirement 1.6.3(4): when
// the directive is received but no FDv1 Fallback Synchronizer is configured, the Data
// System MUST halt — it must not continue attempting FDv2 synchronizers. We use a 500
// status (normally a recoverable error that drives retries) so that if the SDK
// *ignored* the directive, we would see retries on the streaming endpoint; absence of
// those retries is the positive signal that the directive caused a halt rather than
// ordinary permanent removal (which would fire on a 4xx instead).
//
// DATASYSTEM 1.6.3: directive without FDv1 configured halts data system entirely
// CSFDV2 8.1.1: x-ld-fd-fallback header recognized on client-side
func (c CommonStreamingTests) DirectiveWithoutFDv1ConfiguredHaltsDataSystem(t *ldtest.T) {
	handler, channel := httphelpers.RecordingHandler(httphelpers.HandlerWithResponse(
		500, http.Header{"X-LD-FD-Fallback": []string{"true"}}, nil))
	endpoint := requireContext(t).harness.NewMockEndpoint(handler, t.DebugLogger(),
		harness.MockEndpointDescription("streaming service"))
	t.Defer(endpoint.Close)

	// Only a streaming synchronizer is configured; FDv1Fallback is intentionally
	// left unset, which is the "no FDv1 Fallback Synchronizer configured" branch
	// of Requirement 1.6.3(4).
	//
	// A very short retry delay ensures that an SDK that *ignored* the directive
	// would re-contact the streaming endpoint within our observation window —
	// otherwise ordinary backoff could hide a misbehavior.
	_ = NewSDKClient(t,
		WithConfig(servicedef.SDKConfigParams{
			InitCanFail:     true,
			StartWaitTimeMS: o.Some(ldtime.UnixMillisecondTime(1)),
		}),
		WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
			BaseURI:             endpoint.BaseURL(),
			InitialRetryDelayMS: o.Some(briefDelay),
		}))

	// Wait for the first connection so we know the SDK has seen the directive.
	_, err := endpoint.AwaitConnection(time.Second)
	require.NoError(t, err)
	// Drain that first request.
	select {
	case <-channel:
	default:
	}

	// Drain any retry attempts made in the brief window between the directive
	// being observed and the data system actually halting. After that grace period,
	// no further attempts should arrive — if they do, the SDK is still running its
	// ordinary retry/fallback loop instead of halting.
	drainDeadline := time.Now().Add(time.Millisecond * 500)
	for time.Now().Before(drainDeadline) {
		select {
		case <-channel:
		case <-time.After(time.Millisecond * 50):
		}
	}

	h.RequireNever(t, func() bool {
		select {
		case <-channel:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond*20,
		"SDK continued making connections after directive with no FDv1 fallback configured — "+
			"expected the data system to halt per Requirement 1.6.3(4)")
}

// DirectedFallbackIsTerminal verifies Requirement 1.6.4: once the Data System has
// engaged the FDv1 Fallback Synchronizer, it MUST NOT return to the configured FDv2
// synchronizers for the remainder of the SDK's lifetime — no Recovery Condition
// applies. We prove this by keeping the FDv1 endpoint healthy and watching a long
// enough window that the FDv2 Recovery Condition (5 minutes in the default config)
// would normally fire if the SDK were still treating this as a heuristic fallback.
//
// DATASYSTEM 1.6.4: directed fallback is terminal; SDK never returns to FDv2 synchronizers
// CSFDV2 8.1.1: x-ld-fd-fallback triggers client-side FDv1 fallback
// CSFDV2 8.1.3: FDv2 synchronizers remain disabled indefinitely
func (c CommonStreamingTests) DirectedFallbackIsTerminal(t *ldtest.T) {
	t.LongRunning()

	// FDv2 streaming endpoint: 500 + directive on every request. 500 is normally a
	// recoverable error; using it (instead of a 4xx that would permanently remove the
	// sync on its own) means any re-contact we observe must have come from an attempt
	// to *recover* to FDv2 — which 1.6.4 forbids.
	streamHandler, streamChannel := httphelpers.RecordingHandler(httphelpers.HandlerWithResponse(
		500, http.Header{"X-LD-FD-Fallback": []string{"true"}}, nil))
	streamEndpoint := requireContext(t).harness.NewMockEndpoint(streamHandler, t.DebugLogger(),
		harness.MockEndpointDescription("FDv2 streaming service"))
	t.Defer(streamEndpoint.Close)

	// FDv1 fallback endpoint: healthy, serves a distinctive value so we can confirm
	// the SDK is evaluating against FDv1 data (not stale FDv2 data or defaults).
	fdv1Value := ldvalue.String("value-from-fdv1")
	fdv1Handler, _ := httphelpers.RecordingHandler(
		httphelpers.HandlerWithResponse(200, http.Header{"Content-Type": []string{"application/json"}},
			serverSideFDv1AllData(t, c, "flag-key", 1, fdv1Value)))
	fdv1Endpoint := requireContext(t).harness.NewMockEndpoint(fdv1Handler, t.DebugLogger(),
		harness.MockEndpointDescription("FDv1 polling service"))
	t.Defer(fdv1Endpoint.Close)

	client := NewSDKClient(t,
		WithConfig(servicedef.SDKConfigParams{
			StartWaitTimeMS: o.Some(ldtime.UnixMillisecondTime(5 * time.Second / time.Millisecond)),
		}),
		WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
			BaseURI: streamEndpoint.BaseURL(),
		}),
		WithFDv1Fallback(servicedef.SDKConfigPollingParams{
			BaseURI: fdv1Endpoint.BaseURL(),
		}))

	// Wait for the initial streaming connection that surfaces the directive, and drain
	// any recorded requests from the channel. AwaitConnection unblocks as soon as the
	// mock endpoint accepts the connection, which races with the RecordingHandler
	// publishing the request record — a non-blocking select here would leave the
	// initial request in the channel and trip the RequireNever assertion below after
	// the 5-minute sleep. A bounded drain loop gives the recording goroutine time to
	// flush any requests that have already arrived.
	_, err := streamEndpoint.AwaitConnection(time.Second)
	require.NoError(t, err)
	drainDeadline := time.Now().Add(time.Millisecond * 500)
	for time.Now().Before(drainDeadline) {
		select {
		case <-streamChannel:
		case <-time.After(time.Millisecond * 50):
		}
	}

	// Confirm FDv1 is actually serving data before we start waiting — otherwise a
	// later "no FDv2 re-contact" assertion could pass simply because the SDK had
	// already given up on everything.
	context := ldcontext.New("context-key")
	h.RequireEventually(t, func() bool {
		value := basicEvaluateFlag(t, client, "flag-key", context, defaultValue)
		return m.In(t).Assert(value, m.JSONEqual(fdv1Value))
	}, time.Second*3, time.Millisecond*20,
		"flag-key should have been served from the FDv1 fallback before the recovery window begins")

	// Wait past the default Recovery Condition window (5 minutes). Directed Fallback
	// is terminal, so no matter how long we wait the FDv2 streaming endpoint must
	// remain untouched.
	time.Sleep(5*time.Minute + 15*time.Second)

	h.RequireNever(t, func() bool {
		select {
		case <-streamChannel:
			return true
		default:
			return false
		}
	}, time.Millisecond*500, time.Millisecond*20,
		"FDv2 streaming synchronizer was re-contacted after Directed Fallback — "+
			"Directed Fallback must be terminal per Requirement 1.6.4")

	// The FDv1 data must still be the active data after the recovery window —
	// proving the SDK is still operating on FDv1 rather than having silently
	// regressed to defaults or a stale state.
	h.RequireEventually(t, func() bool {
		value := basicEvaluateFlag(t, client, "flag-key", context, defaultValue)
		return m.In(t).Assert(value, m.JSONEqual(fdv1Value))
	}, time.Second*3, time.Millisecond*20,
		"flag-key should still be served from the FDv1 fallback after the recovery window elapses")
}

// DATASYSTEM 1.2.1: synchronizer reconnects with previously known state (basis query param)
func (c CommonStreamingTests) SavesPreviouslyKnownState(t *ldtest.T) {
	dataBefore := c.makeSDKDataWithFlag(1, initialValue)
	dataAfter := mockld.NewServerSDKDataBuilder().IntentCode("none").IntentReason("up-to-date").Build()
	streamEndpoint, _ := makeSequentialStreamHandler(t, dataBefore, dataAfter)
	t.Defer(streamEndpoint.Close)
	client := NewSDKClient(t, WithStreamingSynchronizer(baseStreamConfig(streamEndpoint)))

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue}
	request := validatePayloadReceived(t, streamEndpoint, client, "", expectedEvaluations)
	request.Cancel() // Drop the stream and allow the SDK to reconnect

	validatePayloadReceived(t, streamEndpoint, client, "initial", expectedEvaluations)
}

// DATASYSTEM 1.2.1: xfer-full intent replaces entire store contents on reconnect
func (c CommonStreamingTests) ReplacesPreviouslyKnownState(t *ldtest.T) {
	dataBefore := c.makeSDKDataWithFlag(1, initialValue)
	dataAfter := mockld.NewServerSDKDataBuilder().
		IntentCode("xfer-full").
		IntentReason("cant-catchup").
		Flag(c.makeServerSideFlag("new-flag-key", 1, ldvalue.String("replacement value"))).
		Build()
	streamEndpoint, _ := makeSequentialStreamHandler(t, dataBefore, dataAfter)
	t.Defer(streamEndpoint.Close)
	client := NewSDKClient(t, WithStreamingSynchronizer(baseStreamConfig(streamEndpoint)))

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue, "new-flag-key": defaultValue}
	request := validatePayloadReceived(t, streamEndpoint, client, "", expectedEvaluations)
	request.Cancel() // Drop the stream and allow the SDK to reconnect

	expectedEvaluations = map[string]ldvalue.Value{
		"flag-key":     defaultValue,
		"new-flag-key": ldvalue.String("replacement value"),
	}
	validatePayloadReceived(t, streamEndpoint, client, "initial", expectedEvaluations)
}

// DATASYSTEM 1.2.1: xfer-changes intent applies deltas to existing store on reconnect
func (c CommonStreamingTests) UpdatesPreviouslyKnownState(t *ldtest.T) {
	dataBefore := c.makeSDKDataWithFlag(1, initialValue)
	dataAfter := mockld.NewServerSDKDataBuilder().
		IntentCode("xfer-changes").
		IntentReason("stale").
		Flag(c.makeServerSideFlag("flag-key", 2, updatedValue)).
		Flag(c.makeServerSideFlag("new-flag-key", 1, newInitialValue)).
		Build()
	streamEndpoint, _ := makeSequentialStreamHandler(t, dataBefore, dataAfter)
	t.Defer(streamEndpoint.Close)
	client := NewSDKClient(t, WithStreamingSynchronizer(baseStreamConfig(streamEndpoint)))

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue, "new-flag-key": defaultValue}
	request := validatePayloadReceived(t, streamEndpoint, client, "", expectedEvaluations)
	request.Cancel() // Drop the stream and allow the SDK to reconnect

	expectedEvaluations = map[string]ldvalue.Value{"flag-key": updatedValue, "new-flag-key": newInitialValue}
	validatePayloadReceived(t, streamEndpoint, client, "initial", expectedEvaluations)
}

// DATASYSTEM 1.2.10: updates buffered until payload-transferred event confirms completion
func (c CommonStreamingTests) UpdatesAreNotCompleteUntilPayloadTransferredIsSent(t *ldtest.T) {
	dataSystem, configurers := c.setupDataSystems(t, c.makeSDKDataWithFlag(1, initialValue))
	client := NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)

	_, err := dataSystem.Synchronizers[0].endpoint.AwaitConnection(time.Second)
	require.NoError(t, err)

	context := ldcontext.New("context-key")
	flagKeyValue := basicEvaluateFlag(t, client, "flag-key", context, defaultValue)
	m.In(t).Assert(flagKeyValue, m.JSONEqual(initialValue))

	dataSystem.Synchronizers[0].streaming.PushDelete("flag", "flag-key", 2)
	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", "new-flag-key", 1, c.makeFlagData("new-flag-key", 1, newInitialValue))

	h.RequireNever(
		t,
		checkForUpdatedValue(t, client, "flag-key", context, initialValue, defaultValue, defaultValue),
		time.Millisecond*100,
		time.Millisecond*20,
		"flag value was updated, but it should not have been",
	)

	h.RequireNever(
		t,
		checkForUpdatedValue(t, client, "new-flag-key", context, defaultValue, newInitialValue, defaultValue),
		time.Millisecond*100,
		time.Millisecond*20,
		"flag value was updated, but it should not have been",
	)

	dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 2)

	pollUntilFlagValueUpdated(t, client, "flag-key", context, initialValue, defaultValue, defaultValue)
	pollUntilFlagValueUpdated(t, client, "new-flag-key", context, defaultValue, newInitialValue, defaultValue)
}

// DATASYSTEM 1.2.10: multiple sequential payloads each applied after payload-transferred
func (c CommonStreamingTests) HandlesMultipleUpdates(t *ldtest.T) {
	dataSystem, configurers := c.setupDataSystems(t, c.makeSDKDataWithFlag(1, initialValue))
	client := NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)
	context := ldcontext.New("context-key")

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue}
	validatePayloadReceived(t, dataSystem.Synchronizers[0].Endpoint(), client, "", expectedEvaluations)

	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", "flag-key", 2, c.makeFlagData("flag-key", 2, updatedValue))
	dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 2)
	pollUntilFlagValueUpdated(t, client, "flag-key", context, initialValue, updatedValue, defaultValue)

	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", "new-flag-key", 2, c.makeFlagData("new-flag-key", 2, newInitialValue))
	dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 3)
	pollUntilFlagValueUpdated(t, client, "new-flag-key", context, defaultValue, newInitialValue, defaultValue)

	dataSystem.Synchronizers[0].streaming.PushServerIntent("xfer-full", "stale")
	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", "flag-key", 3, c.makeFlagData("flag-key", 3, finalValue))
	dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 4)
	pollUntilFlagValueUpdated(t, client, "flag-key", context, updatedValue, finalValue, defaultValue)
	pollUntilFlagValueUpdated(t, client, "new-flag-key", context, newInitialValue, defaultValue, defaultValue)
}

// DATASYSTEM 1.2.10: SDK trusts aggregate state version, ignores individual model version
func (c CommonStreamingTests) IgnoresModelVersion(t *ldtest.T) {
	dataSystem, configurers := c.setupDataSystems(t, c.makeSDKDataWithFlag(100, initialValue))
	client := NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)

	expectedEvaluations := map[string]ldvalue.Value{"flag-key": initialValue}
	validatePayloadReceived(t, dataSystem.Synchronizers[0].Endpoint(), client, "", expectedEvaluations)

	// This flag's version is less than the version previously given to the
	// SDK. However, the state we are sending suggests it is later. The SDK
	// should ignore the individual model version and just trust the overall
	// state version.
	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", "flag-key", 1, c.makeFlagData("flag-key", 1, updatedValue))
	dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 2)

	context := ldcontext.New("context-key")
	pollUntilFlagValueUpdated(t, client, "flag-key", context, initialValue, updatedValue, defaultValue)
}

// DATASYSTEM 1.2.2: heartbeat events ignored, do not affect payload processing
func (c CommonStreamingTests) IgnoresHeartBeat(t *ldtest.T) {
	dataSystem, configurers := c.setupDataSystems(t, c.makeSDKDataWithFlag(1, initialValue))
	client := NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)

	_, err := dataSystem.Synchronizers[0].endpoint.AwaitConnection(time.Second)
	require.NoError(t, err)

	context := ldcontext.New("context-key")
	flagKeyValue := basicEvaluateFlag(t, client, "flag-key", context, defaultValue)
	m.In(t).Assert(flagKeyValue, m.JSONEqual(initialValue))

	dataSystem.Synchronizers[0].streaming.PushHeartbeat()
	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", "flag-key", 2, c.makeFlagData("flag-key", 2, updatedValue))
	dataSystem.Synchronizers[0].streaming.PushHeartbeat()
	dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 2)

	pollUntilFlagValueUpdated(t, client, "flag-key", context, initialValue, updatedValue, defaultValue)
}

func (c CommonStreamingTests) IgnoresUnknownEvent(t *ldtest.T) {
	t.Run("unknown event in the middle of a payload", c.IgnoresUnknownEventMiddleOfPayload)
	t.Run("unknown event at the start of a payload", c.IgnoresUnknownEventStartOfPayload)
}

func (c CommonStreamingTests) IgnoresUnknownEventMiddleOfPayload(t *ldtest.T) {
	dataSystem, configurers := c.setupDataSystems(t, c.makeSDKDataWithFlag(1, initialValue))
	client := NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)

	_, err := dataSystem.Synchronizers[0].endpoint.AwaitConnection(time.Second)
	require.NoError(t, err)

	context := ldcontext.New("context-key")
	flagKeyValue := basicEvaluateFlag(t, client, "flag-key", context, defaultValue)
	m.In(t).Assert(flagKeyValue, m.JSONEqual(initialValue))

	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", "flag-key", 2, c.makeFlagData("flag-key", 2, updatedValue))
	dataSystem.Synchronizers[0].streaming.PushEvent("unknown-event-type", map[string]interface{}{
		"some": "data",
	})
	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", "new-flag-key", 1, c.makeFlagData("new-flag-key", 1, newInitialValue))
	dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 2)

	pollUntilFlagValueUpdated(t, client, "flag-key", context, initialValue, updatedValue, defaultValue)
	pollUntilFlagValueUpdated(t, client, "new-flag-key", context, defaultValue, newInitialValue, defaultValue)
}

func (c CommonStreamingTests) IgnoresUnknownEventStartOfPayload(t *ldtest.T) {
	dataSystem, configurers := c.setupDataSystems(t, c.makeSDKDataWithFlag(1, initialValue))
	client := NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)

	_, err := dataSystem.Synchronizers[0].endpoint.AwaitConnection(time.Second)
	require.NoError(t, err)

	context := ldcontext.New("context-key")
	flagKeyValue := basicEvaluateFlag(t, client, "flag-key", context, defaultValue)
	m.In(t).Assert(flagKeyValue, m.JSONEqual(initialValue))

	dataSystem.Synchronizers[0].streaming.PushEvent("unknown-event-type", map[string]interface{}{
		"some": "data",
	})
	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", "flag-key", 2, c.makeFlagData("flag-key", 2, updatedValue))
	dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 2)

	pollUntilFlagValueUpdated(t, client, "flag-key", context, initialValue, updatedValue, defaultValue)
}

// DATASYSTEM 1.2.3: error event discards preceding buffered updates; subsequent payload applied
func (c CommonStreamingTests) CanDiscardPartialEventsOnError(t *ldtest.T) {
	dataSystem, configurers := c.setupDataSystems(t, c.makeSDKDataWithFlag(1, initialValue))
	client := NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)

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

	context := ldcontext.New("context-key")
	h.RequireNever(
		t,
		checkForUpdatedValue(t, client, "flag-key", context, initialValue, updatedValue, defaultValue),
		time.Millisecond*100,
		time.Millisecond*20,
		"flag value was updated, but it should not have been",
	)

	pollUntilFlagValueUpdated(t, client, "new-flag-key", context, defaultValue, newInitialValue, defaultValue)

	// Original flag value should still be the same.
	value := basicEvaluateFlag(t, client, "flag-key", context, defaultValue)
	require.Equal(t, initialValue, value)
}

// DATASYSTEM 1.2.3: error event discards updates; xfer-full intent replaces store entirely
func (c CommonStreamingTests) CanDiscardFullEventsOnError(t *ldtest.T) {
	dataSystem, configurers := c.setupDataSystems(t, c.makeSDKDataWithFlag(1, initialValue))
	client := NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)

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

	context := ldcontext.New("context-key")

	// Previous flag should be removed, reverting to a default value being served.
	pollUntilFlagValueUpdated(t, client, "flag-key", context, initialValue, defaultValue, defaultValue)
	pollUntilFlagValueUpdated(t, client, "new-flag-key", context, defaultValue, newInitialValue, defaultValue)
}

// DATASYSTEM 1.2.2: goodbye event causes SDK to disconnect and discard pending updates
func (c CommonStreamingTests) DisconnectsOnGoodbye(t *ldtest.T) {
	dataBefore := c.makeSDKDataWithFlag(1, initialValue)
	dataAfter := mockld.NewServerSDKDataBuilder().IntentCode("none").IntentReason("up-to-date").Build()
	streamEndpoint, dataSystems := makeSequentialStreamHandler(t, dataBefore, dataAfter)
	t.Defer(streamEndpoint.Close)
	client := NewSDKClient(t, WithStreamingSynchronizer(baseStreamConfig(streamEndpoint)))

	conn := streamEndpoint.RequireConnection(t, time.Second)

	dataSystems[0].Synchronizers[0].streaming.PushUpdate(
		"flag", "flag-key", 2, c.makeFlagData("flag-key", 2, updatedValue))
	// This should prompt the SDK to discard previous events, disconnect, and then re-connect.
	dataSystems[0].Synchronizers[0].streaming.PushGoodbye("some-reason", false, false)
	conn.Cancel()

	_ = streamEndpoint.RequireConnection(t, time.Second)

	context := ldcontext.New("context-key")
	h.RequireNever(
		t,
		checkForUpdatedValue(t, client, "flag-key", context, initialValue, updatedValue, defaultValue),
		time.Millisecond*100,
		time.Millisecond*20,
		"flag value was updated, but it should not have been",
	)
}

func makeSequentialStreamHandler(t *ldtest.T, dataSources ...mockld.SDKData) (
	*harness.MockEndpoint, []*SDKDataSystem,
) {
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
	state string, evaluations map[string]ldvalue.Value,
) harness.IncomingRequestInfo {
	request, err := streamEndpoint.AwaitConnection(time.Second)
	require.NoError(t, err)

	m.In(t).Assert(request.URL.Query().Get("basis"), m.Equal(state))

	context := ldcontext.New("context-key")

	h.RequireEventually(t, func() bool {
		for flagKey, expectedValue := range evaluations {
			actualValue := basicEvaluateFlag(t, client, flagKey, context, defaultValue)
			if !m.In(t).Assert(actualValue, m.JSONEqual(expectedValue)) {
				return false
			}
		}

		return true
	}, time.Second, time.Millisecond*20, "failed to evaluate flag")

	return request
}
