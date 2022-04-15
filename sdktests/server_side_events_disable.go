package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

func doServerSideEventDisableTest(t *ldtest.T) {
	// We can only do this test if the SDK allows us to say "set the events base URI to ____" and "don't send
	// events" at the same time; otherwise there would be no way to verify that events were not sent to our
	// mock endpoint.
	t.RequireCapability(servicedef.CapabilityServiceEndpoints)

	context := ldcontext.New("user-key")

	doTest := func(t *ldtest.T, name string, actionThatCausesEvent func(*ldtest.T, *SDKClient)) {
		t.Run(name, func(t *ldtest.T) {
			dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
			events := NewSDKEventSink(t)
			client := NewSDKClient(t,
				WithServiceEndpointsConfig(servicedef.SDKConfigServiceEndpointsParams{
					Events: events.Endpoint().BaseURL(),
				}),
				dataSource)

			actionThatCausesEvent(t, client)
			client.FlushEvents(t)

			events.ExpectNoAnalyticsEvents(t, time.Millisecond*100)
		})
	}

	doTest(t, "evaluation", func(t *ldtest.T, client *SDKClient) {
		_ = basicEvaluateFlag(t, client, "nonexistent-flag", context, ldvalue.Null())
	})

	doTest(t, "identify event", func(t *ldtest.T, client *SDKClient) {
		client.SendIdentifyEvent(t, context)
	})

	doTest(t, "custom event", func(t *ldtest.T, client *SDKClient) {
		client.SendCustomEvent(t, servicedef.CustomEventParams{
			EventKey: "event-key",
			Context:  o.Some(context),
		})
	})
}
