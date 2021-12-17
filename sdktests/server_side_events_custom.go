package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"
	"github.com/launchdarkly/sdk-test-harness/testmodel"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/require"
)

func doServerSideCustomEventTests(t *ldtest.T) {
	// These do not include detailed tests of the encoding of user attributes in custom events,
	// which are in server_side_events_users.go.
	sources, err := testmodel.ReadAllFiles("testdata/custom-events")
	require.NoError(t, err)

	eventsConfig := baseEventsConfig()
	eventsConfig.InlineUsers = true // so we don't get index events in the output

	dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{Events: &eventsConfig}), dataSource, events)

	for _, source := range sources {
		var suite testmodel.CustomEventTestSuite
		require.NoError(t, source.ParseInto(&suite))

		t.Run(source.ParamsString(), func(t *ldtest.T) {
			for _, test := range suite.Events {
				client.SendCustomEvent(t, servicedef.CustomEventParams{
					EventKey:     test.EventKey,
					User:         &test.User,
					Data:         test.Data,
					OmitNullData: test.OmitNullData,
					MetricValue:  test.MetricValue,
				})
				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.AssertThat(t, payload, m.ItemsInAnyOrder(
					EventIsCustomEvent(test.EventKey, mockld.SimpleEventUser(test.User), true, test.Data, test.MetricValue),
				))
			}
		})
	}
}
