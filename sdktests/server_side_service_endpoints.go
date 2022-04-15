package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
)

func doServerSideServiceEndpointsTests(t *ldtest.T) {
	// These tests verify at a very basic level that the SDK can be configured to use custom
	// service base URIs. If it can't, then pretty much *all* of our tests will fail, but at
	// least the fact that these particular tests also fail might make the fundamental problem
	// easier to diagnose.

	// In some SDKs, these URIs can only be set as part of the configuration for a specific
	// service; in others, they are set separate; or both. There is a test for each mode here
	// even though the test service may end up doing the same thing for both.

	doTest := func(
		t *ldtest.T,
		makeStreamingConfig func(*SDKDataSource) SDKConfigurer,
		makeEventsConfig func(*SDKEventSink) SDKConfigurer,
	) {
		t.Run("streaming", func(t *ldtest.T) {
			dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
			_ = NewSDKClient(
				t,
				makeStreamingConfig(dataSource),
			)
			_ = dataSource.Endpoint().RequireConnection(t, time.Second)
		})

		t.Run("events", func(t *ldtest.T) {
			dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
			events := NewSDKEventSink(t)
			client := NewSDKClient(
				t,
				makeEventsConfig(events),
				dataSource,
			)
			client.SendIdentifyEvent(t, lduser.NewUser("user-key"))
			client.FlushEvents(t)
			_ = events.Endpoint().RequireConnection(t, time.Second)
		})
	}

	t.Run("using per-component configuration", func(t *ldtest.T) {
		doTest(
			t,
			func(dataSource *SDKDataSource) SDKConfigurer {
				return WithStreamingConfig(servicedef.SDKConfigStreamingParams{
					BaseURI: dataSource.Endpoint().BaseURL(),
				})
			},
			func(events *SDKEventSink) SDKConfigurer {
				return WithEventsConfig(servicedef.SDKConfigEventParams{
					BaseURI: events.Endpoint().BaseURL(),
				})
			},
		)
	})

	t.Run("using separate service endpoints properties", func(t *ldtest.T) {
		t.RequireCapability(servicedef.CapabilityServiceEndpoints)
		doTest(
			t,
			func(dataSource *SDKDataSource) SDKConfigurer {
				return WithServiceEndpointsConfig(servicedef.SDKConfigServiceEndpointsParams{
					Streaming: dataSource.Endpoint().BaseURL(),
				})
			},
			func(events *SDKEventSink) SDKConfigurer {
				return WithConfig(servicedef.SDKConfigParams{
					Events: o.Some(servicedef.SDKConfigEventParams{}),
					ServiceEndpoints: o.Some(servicedef.SDKConfigServiceEndpointsParams{
						Events: events.Endpoint().BaseURL(),
					}),
				})
			},
		)
	})
}
