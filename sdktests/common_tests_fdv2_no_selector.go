package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/v3/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/require"
)

// NoSelector covers full payloads without a selector, as served by the Relay Proxy from a persistent
// store. They initialize the SDK but do not provide a basis, and clear any previously held selector.
func (c CommonStreamingTests) NoSelector(t *ldtest.T) {
	t.Run("stream initializes", c.NoSelectorStreamInitializes)
	t.Run("stream reconnects without a basis", c.NoSelectorStreamReconnectsWithoutBasis)
	t.Run("stream clears a previously held selector", c.NoSelectorStreamClearsPreviouslyHeldSelector)
	t.Run("polling synchronizer initializes", c.NoSelectorPollingSynchronizerInitializes)
	t.Run("polling initializer is followed by a synchronizer request without a basis",
		c.NoSelectorPollingInitializerThenSynchronizerWithoutBasis)
}

func (c CommonStreamingTests) noSelectorData(value ldvalue.Value) mockld.FDv2SDKData {
	if c.isClientSide {
		return c.fdv2ClientData("xfer-full", "cant-catchup", "", c.makeClientSideFlag("flag-key", 1, value))
	}
	return c.fdv2ServerData("xfer-full", "cant-catchup", "", c.makeServerSideFlag("flag-key", 1, value))
}

func requireNoBasisParam(t *ldtest.T, request harness.IncomingRequestInfo) {
	t.Helper()
	m.In(t).For("basis query parameter present").
		Assert(request.URL.Query().Has("basis"), m.Equal(false))
}

func (c CommonStreamingTests) NoSelectorStreamClearsPreviouslyHeldSelector(t *ldtest.T) {
	streamEndpoint, dataSystems := makeSequentialStreamHandler(t,
		c.makeSDKDataWithFlag(1, initialValue), c.noSelectorData(finalValue))
	t.Defer(streamEndpoint.Close)
	client := c.newFDv2SDKClient(t, WithStreamingSynchronizer(reconnectStateTestStreamConfig(streamEndpoint)))

	request := validatePayloadReceived(t, streamEndpoint, client, c.flagEvaluationContext, "",
		map[string]ldvalue.Value{"flag-key": initialValue})
	streaming := dataSystems[0].Synchronizers[0].streaming
	streaming.SetInitialData(c.noSelectorData(updatedValue))
	streaming.RefreshAll()
	pollUntilFlagValueUpdated(t, client, "flag-key", c.flagEvaluationContext,
		initialValue, updatedValue, defaultValue)
	request.Cancel()

	request = validatePayloadReceived(t, streamEndpoint, client, c.flagEvaluationContext, "",
		map[string]ldvalue.Value{"flag-key": finalValue})
	requireNoBasisParam(t, request)
}

func (c CommonStreamingTests) NoSelectorStreamInitializes(t *ldtest.T) {
	dataSystem, configurers := c.setupDataSystems(t, c.noSelectorData(initialValue))
	client := c.newFDv2SDKClient(t, configurers...)

	validatePayloadReceived(t, dataSystem.Synchronizers[0].Endpoint(), client,
		c.flagEvaluationContext, "", map[string]ldvalue.Value{"flag-key": initialValue})
}

func (c CommonStreamingTests) NoSelectorStreamReconnectsWithoutBasis(t *ldtest.T) {
	streamEndpoint, _ := makeSequentialStreamHandler(t,
		c.noSelectorData(initialValue), c.noSelectorData(updatedValue))
	t.Defer(streamEndpoint.Close)
	client := c.newFDv2SDKClient(t, WithStreamingSynchronizer(reconnectStateTestStreamConfig(streamEndpoint)))

	request := validatePayloadReceived(t, streamEndpoint, client, c.flagEvaluationContext, "",
		map[string]ldvalue.Value{"flag-key": initialValue})
	request.Cancel()

	request = validatePayloadReceived(t, streamEndpoint, client, c.flagEvaluationContext, "",
		map[string]ldvalue.Value{"flag-key": updatedValue})
	requireNoBasisParam(t, request)
}

func (c CommonStreamingTests) NoSelectorPollingSynchronizerInitializes(t *ldtest.T) {
	var options []SDKDataSystemOption
	if c.isClientSide {
		options = []SDKDataSystemOption{
			DataSystemOptionConnectionMode("polling", DataSystemOptionPolling()),
			DataSystemOptionInitialConnectionMode("polling"),
		}
	} else {
		options = []SDKDataSystemOption{DataSystemOptionPolling()}
	}
	dataSystem := NewSDKDataSystemCustom(t, c.noSelectorData(initialValue), options...)
	dataSystem.CreateEndpoints()
	client := c.newFDv2SDKClient(t, dataSystem)

	request := validatePayloadReceived(t, dataSystem.Synchronizers[0].Endpoint(), client,
		c.flagEvaluationContext, "", map[string]ldvalue.Value{"flag-key": initialValue})
	requireNoBasisParam(t, request)
}

func (c CommonStreamingTests) NoSelectorPollingInitializerThenSynchronizerWithoutBasis(t *ldtest.T) {
	dataSystem := NewSDKDataSystemCustom(t, c.noSelectorData(updatedValue),
		DataSystemOptionPollingInitializer(c.noSelectorData(initialValue)), DataSystemOptionStreaming())
	dataSystem.CreateEndpoints()
	client := c.newFDv2SDKClient(t, dataSystem)

	_, err := dataSystem.Initializers[0].Endpoint().AwaitConnection(time.Second)
	require.NoError(t, err)

	request := validatePayloadReceived(t, dataSystem.Synchronizers[0].Endpoint(), client,
		c.flagEvaluationContext, "", map[string]ldvalue.Value{"flag-key": updatedValue})
	requireNoBasisParam(t, request)
}
