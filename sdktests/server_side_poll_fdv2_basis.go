package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

// doServerSidePollFDv2BasisTests verifies the basis query parameter that an FDv2 polling SDK
// sends. The SDK sends the state from the selector of the most recent payload-transferred
// event as the basis query parameter on its polling requests.
func doServerSidePollFDv2BasisTests(t *ldtest.T) {
	sdkKey := "my-sdk-key"

	pollTests := NewCommonPollingTests(t, "doServerSidePollFDv2BasisTests",
		WithConfig(servicedef.SDKConfigParams{
			Credential: sdkKey,
		}))

	pollTests.FDv2RequestSendsBasisSelector(t)
}

// FDv2RequestSendsBasisSelector checks that the polling synchronizer sends the basis query
// parameter.
//
// A polling initializer seeds the SDK with a selector at startup. That selector carries the
// state from the initializer's payload-transferred event. The synchronizer's first poll must
// therefore send that state as the basis query parameter.
func (c CommonPollingTests) FDv2RequestSendsBasisSelector(t *ldtest.T) {
	t.Run("sends the basis query parameter from the selector", func(t *ldtest.T) {
		const basisState = "poll-basis-state"

		// Both the initializer and the synchronizer serve a payload-transferred event whose
		// state is basisState.
		sdkData := mockld.FDv2SDKDataFromServerSDKData(
			mockld.NewServerSDKDataBuilder().Build(),
			"xfer-full", "initial", basisState,
		)

		dataSystem := NewSDKDataSystemCustom(t, sdkData,
			DataSystemOptionPollingInitializer(sdkData),
			DataSystemOptionPolling())
		dataSystem.CreateEndpoints()

		_ = NewSDKClient(t, c.baseSDKConfigurationPlus(dataSystem)...)

		// Allow a generous timeout to cover SDK startup and the initializer handoff.
		request := dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second*15)
		m.In(t).For("basis query parameter").
			Assert(request.URL.Query().Get("basis"), m.Equal(basisState))
	})
}
