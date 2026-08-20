package sdktests

import (
	"time"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"
)

func doClientSideSecureModeHashTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilitySecureModeHash)

	const testHash = "test-secure-mode-hash"

	t.Run("streaming", func(t *ldtest.T) {
		t.Run("sends h query parameter when hash is configured", func(t *ldtest.T) {
			c := NewCommonStreamingTests(t, "doClientSideSecureModeHashTests")
			dataSystem, configurers := c.setupDataSystems(t, nil)

			_ = NewSDKClient(t, append(
				configurers,
				WithClientSideConfig(servicedef.SDKConfigClientSideParams{
					InitialContext: o.Some(c.contextFactory.NextUniqueContext()),
					Hash:           o.Some(testHash),
				}),
			)...)

			request := dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)
			m.In(t).For("h query parameter").Assert(request.URL.RawQuery,
				UniqueQueryParameters().Should(m.MapIncluding(m.KV("h", m.Equal(testHash)))))
		})

		t.Run("omits h query parameter when hash is not configured", func(t *ldtest.T) {
			c := NewCommonStreamingTests(t, "doClientSideSecureModeHashTests")
			dataSystem, configurers := c.setupDataSystems(t, nil)

			_ = NewSDKClient(t, append(
				configurers,
				WithClientSideConfig(servicedef.SDKConfigClientSideParams{
					InitialContext: o.Some(c.contextFactory.NextUniqueContext()),
				}),
			)...)

			request := dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)
			m.In(t).For("h query parameter").Assert(request.URL.RawQuery,
				UniqueQueryParameters().Should(m.Not(MapHasKey("h"))))
		})
	})

	t.Run("polling", func(t *ldtest.T) {
		t.Run("sends h query parameter when hash is configured", func(t *ldtest.T) {
			p := NewCommonPollingTests(t, "doClientSideSecureModeHashTests")
			dataSystem := NewSDKDataSystem(t, nil, p.pollingDataSystemOptions()...)

			_ = NewSDKClient(t,
				WithClientSideConfig(servicedef.SDKConfigClientSideParams{
					InitialContext: o.Some(p.contextFactory.NextUniqueContext()),
					Hash:           o.Some(testHash),
				}),
				dataSystem,
			)

			request := dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)
			m.In(t).For("h query parameter").Assert(request.URL.RawQuery,
				UniqueQueryParameters().Should(m.MapIncluding(m.KV("h", m.Equal(testHash)))))
		})

		t.Run("omits h query parameter when hash is not configured", func(t *ldtest.T) {
			p := NewCommonPollingTests(t, "doClientSideSecureModeHashTests")
			dataSystem := NewSDKDataSystem(t, nil, p.pollingDataSystemOptions()...)

			_ = NewSDKClient(t,
				WithClientSideConfig(servicedef.SDKConfigClientSideParams{
					InitialContext: o.Some(p.contextFactory.NextUniqueContext()),
				}),
				dataSystem,
			)

			request := dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)
			m.In(t).For("h query parameter").Assert(request.URL.RawQuery,
				UniqueQueryParameters().Should(m.Not(MapHasKey("h"))))
		})
	})
}
