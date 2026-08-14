package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

// doServerSidePollFDv2BasisTests verifies the basis query parameter that an FDv2 polling SDK
// sends. The parameter carries the state from the selector of the most recent
// payload-transferred event.
func doServerSidePollFDv2BasisTests(t *ldtest.T) {
	sdkKey := "my-sdk-key"

	pollTests := NewCommonPollingTests(t, "doServerSidePollFDv2BasisTests",
		WithConfig(servicedef.SDKConfigParams{
			Credential: sdkKey,
		}))

	pollTests.FDv2RequestSendsBasisSelector(t)
}

// FDv2RequestSendsBasisSelector checks the basis query parameter across two consecutive polls.
//
// The first poll happens before the SDK has a selector, so it must not send basis. Each poll
// response carries a payload-transferred event with a known state, so the next poll must send
// that state as the basis query parameter.
func (c CommonPollingTests) FDv2RequestSendsBasisSelector(t *ldtest.T) {
	t.Run("sends basis query parameter on subsequent polls", func(t *ldtest.T) {
		// The second poll only happens after the polling interval elapses. Server-side SDKs
		// clamp the interval to 30 seconds, so waiting for it makes this test long-running.
		t.LongRunning()

		const basisState = "poll-basis-state"

		// Every poll response carries a payload-transferred event whose state is basisState.
		sdkData := mockld.FDv2SDKDataFromServerSDKData(
			mockld.NewServerSDKDataBuilder().Build(),
			"xfer-full", "initial", basisState,
		)

		options := append(c.pollingDataSystemOptions(), DataSystemOptionPollInterval(time.Second))
		dataSystem := NewSDKDataSystem(t, sdkData, options...)

		_ = NewSDKClient(t, c.baseSDKConfigurationPlus(dataSystem)...)

		endpoint := dataSystem.Synchronizers[0].Endpoint()

		// Allow a generous timeout for the first request to cover SDK startup.
		firstRequest := endpoint.RequireConnection(t, time.Second*15)
		m.In(t).For("basis query parameter on first poll").
			Assert(firstRequest.URL.Query().Get("basis"), m.Equal(""))

		// The second poll must send the state from the first response as the basis parameter.
		// The timeout covers the clamped 30-second interval plus SDK jitter.
		secondRequest := endpoint.RequireConnection(t, time.Second*35)
		m.In(t).For("basis query parameter on second poll").
			Assert(secondRequest.URL.Query().Get("basis"), m.Equal(basisState))
	})
}
