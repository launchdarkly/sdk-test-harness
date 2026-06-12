package sdktests

import (
	"time"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

func doClientSideSecureModeHashTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilitySecureModeHash)

	const testHash = "test-secure-mode-hash"

	t.Run("streaming", func(t *ldtest.T) {
		t.Run("sends h query parameter when hash is configured", func(t *ldtest.T) {
			c := NewCommonStreamingTests(t, "doClientSideSecureModeHashTests")
			dataSource, configurers := c.setupDataSources(t, nil)

			_ = NewSDKClient(t, append(
				configurers,
				WithClientSideConfig(servicedef.SDKConfigClientSideParams{
					InitialContext: o.Some(c.contextFactory.NextUniqueContext()),
					Hash:           o.Some(testHash),
				}),
			)...)

			request := dataSource.Endpoint().RequireConnection(t, time.Second)
			m.In(t).For("h query parameter").Assert(request.URL.RawQuery,
				UniqueQueryParameters().Should(m.MapIncluding(m.KV("h", m.Equal(testHash)))))

			// JS client SDKs connect to the polling service first in streaming mode. Assert h is
			// present on that bootstrap request too; configurers[0] is the polling data source.
			if c.sdkKind == mockld.JSClientSDK {
				bootstrapDS := configurers[0].(*SDKDataSource)
				bootstrapRequest := bootstrapDS.Endpoint().RequireConnection(t, time.Second)
				m.In(t).For("h query parameter on bootstrap poll").Assert(bootstrapRequest.URL.RawQuery,
					UniqueQueryParameters().Should(m.MapIncluding(m.KV("h", m.Equal(testHash)))))
			}
		})

		t.Run("omits h query parameter when hash is not configured", func(t *ldtest.T) {
			c := NewCommonStreamingTests(t, "doClientSideSecureModeHashTests")
			dataSource, configurers := c.setupDataSources(t, nil)

			_ = NewSDKClient(t, append(
				configurers,
				WithClientSideConfig(servicedef.SDKConfigClientSideParams{
					InitialContext: o.Some(c.contextFactory.NextUniqueContext()),
				}),
			)...)

			request := dataSource.Endpoint().RequireConnection(t, time.Second)
			// m.AllOf() is used as "match any value" since this matchers library has no Anything() function.
			m.In(t).For("h query parameter").Assert(request.URL.RawQuery,
				UniqueQueryParameters().Should(m.Not(m.MapIncluding(m.KV("h", m.AllOf())))))

			// JS client SDKs connect to the polling service first in streaming mode. Assert h is
			// absent on that bootstrap request too.
			if c.sdkKind == mockld.JSClientSDK {
				bootstrapDS := configurers[0].(*SDKDataSource)
				bootstrapRequest := bootstrapDS.Endpoint().RequireConnection(t, time.Second)
				m.In(t).For("h query parameter on bootstrap poll").Assert(bootstrapRequest.URL.RawQuery,
					UniqueQueryParameters().Should(m.Not(m.MapIncluding(m.KV("h", m.AllOf())))))
			}
		})
	})

	t.Run("polling", func(t *ldtest.T) {
		t.Run("sends h query parameter when hash is configured", func(t *ldtest.T) {
			dataSource := NewSDKDataSource(t, nil, DataSourceOptionPolling())
			contextFactory := data.NewContextFactory("doClientSideSecureModeHashTests")

			_ = NewSDKClient(t,
				WithClientSideConfig(servicedef.SDKConfigClientSideParams{
					InitialContext: o.Some(contextFactory.NextUniqueContext()),
					Hash:           o.Some(testHash),
				}),
				dataSource,
			)

			request := dataSource.Endpoint().RequireConnection(t, time.Second)
			m.In(t).For("h query parameter").Assert(request.URL.RawQuery,
				UniqueQueryParameters().Should(m.MapIncluding(m.KV("h", m.Equal(testHash)))))
		})

		t.Run("omits h query parameter when hash is not configured", func(t *ldtest.T) {
			dataSource := NewSDKDataSource(t, nil, DataSourceOptionPolling())
			contextFactory := data.NewContextFactory("doClientSideSecureModeHashTests")

			_ = NewSDKClient(t,
				WithClientSideConfig(servicedef.SDKConfigClientSideParams{
					InitialContext: o.Some(contextFactory.NextUniqueContext()),
				}),
				dataSource,
			)

			request := dataSource.Endpoint().RequireConnection(t, time.Second)
			// m.AllOf() with no args is vacuously true; used as "match any value" since this
			// matchers library has no Anything() function.
			m.In(t).For("h query parameter").Assert(request.URL.RawQuery,
				UniqueQueryParameters().Should(m.Not(m.MapIncluding(m.KV("h", m.AllOf())))))
		})
	})
}
