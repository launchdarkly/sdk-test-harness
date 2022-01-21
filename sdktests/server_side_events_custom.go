package sdktests

import (
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

func doServerSideCustomEventTests(t *ldtest.T) {
	// These do not include detailed tests of the encoding of user attributes in custom events,
	// which are in server_side_events_users.go.

	t.Run("data and metricValue parameters", doServerSideParameterizedCustomEventTests)

	t.Run("index events are created for new users if not inlined", func(t *ldtest.T) {
		eventsConfig := baseEventsConfig()

		dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
		events := NewSDKEventSink(t)
		client := NewSDKClient(t, dataSource, events)

		user1 := lduser.NewUserBuilder("user1").Name("Ann").Build()
		user2 := lduser.NewUserBuilder("user2").Name("Bob").Build()
		user3 := lduser.NewUserBuilder("user3").Name("Cat").Build()

		event1 := servicedef.CustomEventParams{EventKey: "event1", User: &user1} // generates index event for user1
		event2 := servicedef.CustomEventParams{EventKey: "event2", User: &user1} // no new index, user1 already seen
		event3 := servicedef.CustomEventParams{EventKey: "event3", User: &user2} // generates index event for user2
		event4 := servicedef.CustomEventParams{EventKey: "event4", User: &user1} // no new index, user1 already seen
		event5 := servicedef.CustomEventParams{EventKey: "event5", User: &user2} // no new index, user2 already seen
		event6 := servicedef.CustomEventParams{EventKey: "event6", User: &user3} // generates index event for user3

		for _, params := range []servicedef.CustomEventParams{event1, event2, event3, event4, event5, event6} {
			client.SendCustomEvent(t, params)
		}
		client.FlushEvents(t)
		payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		expected := make([]m.Matcher, 0)
		for _, params := range []servicedef.CustomEventParams{event1, event2, event3, event4, event5, event6} {
			expected = append(expected, EventIsCustomEventForParams(params, eventsConfig))
		}
		for _, user := range []lduser.User{user1, user2, user3} {
			expected = append(expected, EventIsIndexEvent(mockld.ExpectedEventUserFromUser(user, eventsConfig)))
		}
		m.In(t).Assert(payload, m.ItemsInAnyOrder(expected...))
	})
}

func doServerSideParameterizedCustomEventTests(t *ldtest.T) {
	eventsConfig := baseEventsConfig()
	eventsConfig.InlineUsers = true // so we don't get index events in the output

	dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{Events: &eventsConfig}), dataSource, events)

	user := lduser.NewUser("user-key")

	// Generate many permutations of 1. data types that can be used for the data parameter, if any, and
	// 2. metric value parameter, if any.
	allParams := make([]servicedef.CustomEventParams, 0)
	omitMetricValue := float64(-999999) // magic value that we'll change to null
	for _, metricValue := range []float64{
		omitMetricValue,
		0,
		-1.5,
		1.5,
	} {
		baseParams := servicedef.CustomEventParams{
			EventKey: "event-key",
			User:     &user,
		}
		if metricValue != omitMetricValue {
			m := metricValue
			baseParams.MetricValue = &m
		}

		for _, dataValue := range []ldvalue.Value{
			ldvalue.Null(),
			ldvalue.Bool(false),
			ldvalue.Bool(true),
			ldvalue.Int(0),
			ldvalue.Int(1000),
			ldvalue.Float64(1000.5),
			ldvalue.String(""),
			ldvalue.String("abc"),
			ldvalue.ArrayOf(ldvalue.Int(1), ldvalue.Int(2)),
			ldvalue.ObjectBuild().Set("property", ldvalue.Bool(true)).Build(),
		} {
			params := baseParams
			params.Data = dataValue
			allParams = append(allParams, params)
		}

		// Add another case where the data parameter is null and we also set omitNullData. This is a
		// hint to the test service for SDKs that may have a different API for "no data" than "optional
		// data which may be null", to make sure we're covering both methods.
		params := baseParams
		params.OmitNullData = true
		allParams = append(allParams, params)
	}

	for _, params := range allParams {
		desc := fmt.Sprintf("data=%s", params.Data.JSONString())
		if params.OmitNullData {
			desc += ", omitNullData"
		}
		if params.MetricValue != nil {
			desc += fmt.Sprintf(", metricValue=%f", *params.MetricValue)
		}

		t.Run(desc, func(t *ldtest.T) {
			client.SendCustomEvent(t, params)
			client.FlushEvents(t)
			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
			m.In(t).Assert(payload, m.Items(
				EventIsCustomEventForParams(params, eventsConfig),
			))
		})
	}
}
