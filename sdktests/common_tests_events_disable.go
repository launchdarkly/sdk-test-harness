package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

func (c CommonEventTests) DisablingEvents(t *ldtest.T) {
	// We can only do this test if the SDK allows us to say "set the events base URI to ____" and "don't send
	// events" at the same time; otherwise there would be no way to verify that events were not sent to our
	// mock endpoint.
	t.RequireCapability(servicedef.CapabilityServiceEndpoints)

	user := lduser.NewUser("user-key")

	doTest := func(t *ldtest.T, name string, actionThatCausesEvent func(*ldtest.T, *SDKClient)) {
		t.Run(name, func(t *ldtest.T) {
			dataSource := NewSDKDataSource(t, nil)
			events := NewSDKEventSink(t)
			client := NewSDKClient(t, c.baseSDKConfigurationPlus(
				WithServiceEndpointsConfig(servicedef.SDKConfigServiceEndpointsParams{
					Events: events.Endpoint().BaseURL(),
				}),
				dataSource)...)

			actionThatCausesEvent(t, client)
			client.FlushEvents(t)

			events.ExpectNoAnalyticsEvents(t, time.Millisecond*100)
		})
	}

	doTest(t, "evaluation", func(t *ldtest.T, client *SDKClient) {
		_ = basicEvaluateFlag(t, client, "nonexistent-flag", user, ldvalue.Null())
	})

	doTest(t, "identify event", func(t *ldtest.T, client *SDKClient) {
		client.SendIdentifyEvent(t, user)
	})

	doTest(t, "custom event", func(t *ldtest.T, client *SDKClient) {
		client.SendCustomEvent(t, servicedef.CustomEventParams{
			EventKey: "event-key",
			User:     o.Some(user),
		})
	})

	doTest(t, "alias event", func(t *ldtest.T, client *SDKClient) {
		client.SendAliasEvent(t, servicedef.AliasEventParams{
			User:         user,
			PreviousUser: lduser.NewUser("previous-user-key"),
		})
	})
}
