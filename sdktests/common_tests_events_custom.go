package sdktests

import (
	"fmt"
	"time"

	h "github.com/launchdarkly/sdk-test-harness/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

func (c CommonEventTests) CustomEvents(t *ldtest.T) {
	// These do not include detailed tests of the encoding of user attributes in custom events,
	// which are in common_tests_events_users.go.

	t.Run("data and metricValue parameters", c.customEventsParameterizedTests)

	t.Run("basic properties", func(t *ldtest.T) {
		metricValue := 1.0

		baseCustomEventProperties := []string{
			"kind", "contextKind", "creationDate", "key", "data", "metricValue",
		}
		if t.Capabilities().Has(servicedef.CapabilityClientSide) &&
			!t.Capabilities().Has(servicedef.CapabilityMobile) {
			// this is a JS-based client-side SDK, so custom events may have an additional property
			baseCustomEventProperties = append(baseCustomEventProperties, "url")
		}

		for _, inlineUser := range []bool{false, true} {
			t.Run(h.IfElse(inlineUser, "inline user", "non-inline user"), func(t *ldtest.T) {
				for _, anonymousUser := range []bool{false, true} {
					t.Run(h.IfElse(anonymousUser, "anonymous user", "non-anonymous user"), func(t *ldtest.T) {
						eventsConfig := servicedef.SDKConfigEventParams{InlineUsers: inlineUser}
						user := c.userFactory.NextUniqueUserMaybeAnonymous(anonymousUser)

						dataSource := NewSDKDataSource(t, nil)
						events := NewSDKEventSink(t)
						client := NewSDKClient(t, c.baseSDKConfigurationPlus(WithEventsConfig(eventsConfig), dataSource, events)...)

						if c.isClientSide {
							client.SendIdentifyEvent(t, user)
							client.FlushEvents(t)
							_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout)
						}
						client.SendCustomEvent(t, servicedef.CustomEventParams{
							EventKey:    "event-key",
							User:        o.Some(user),
							Data:        ldvalue.Bool(true),
							MetricValue: o.Some(metricValue),
						})

						client.FlushEvents(t)
						payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

						if inlineUser {
							expectedEvent := m.AllOf(
								JSONPropertyKeysCanOnlyBe(append(baseCustomEventProperties, "user")...),
								IsCustomEvent(),
								HasUserObjectWithKey(user.GetKey()),
								HasContextKind(user),
							)
							m.In(t).Assert(payload, m.Items(expectedEvent))
						} else {
							expectedEvents := []m.Matcher{}
							if !c.isClientSide {
								expectedEvents = append(expectedEvents, IsIndexEvent())
							}
							expectedEvents = append(expectedEvents, m.AllOf(
								JSONPropertyKeysCanOnlyBe(append(baseCustomEventProperties, "userKey")...),
								IsCustomEvent(),
								HasUserKeyProperty(user.GetKey()),
								HasContextKind(user),
							))
							m.In(t).Assert(payload, m.ItemsInAnyOrder(expectedEvents...))
						}
					})
				}
			})
		}
	})
}

func (c CommonEventTests) customEventsParameterizedTests(t *ldtest.T) {
	eventsConfig := baseEventsConfig()
	eventsConfig.InlineUsers = true // so we don't get index events in the output

	dataSource := NewSDKDataSource(t, nil)
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, c.baseSDKConfigurationPlus(WithEventsConfig(eventsConfig), dataSource, events)...)

	if c.isClientSide {
		// ignore the initial identify event
		client.FlushEvents(t)
		_ = events.ExpectAnalyticsEvents(t, time.Second)
	}

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
		}
		if metricValue != omitMetricValue {
			baseParams.MetricValue = o.Some(metricValue)
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
			params.User = o.Some(c.userFactory.NextUniqueUser())
			allParams = append(allParams, params)
		}

		// Add another case where the data parameter is null and we also set omitNullData. This is a
		// hint to the test service for SDKs that may have a different API for "no data" than "optional
		// data which may be null", to make sure we're covering both methods.
		params := baseParams
		params.OmitNullData = true
		params.User = o.Some(c.userFactory.NextUniqueUser())
		allParams = append(allParams, params)
	}

	for _, params := range allParams {
		desc := fmt.Sprintf("data=%s", params.Data.JSONString())
		if params.OmitNullData {
			desc += ", omitNullData"
		}
		if params.MetricValue.IsDefined() {
			desc += fmt.Sprintf(", metricValue=%f", params.MetricValue.Value())
		}

		t.Run(desc, func(t *ldtest.T) {
			client.SendCustomEvent(t, params)
			client.FlushEvents(t)
			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
			m.In(t).Assert(payload, m.Items(
				m.AllOf(
					IsCustomEventForEventKey(params.EventKey),
					h.IfElse(params.OmitNullData && params.Data.IsNull(),
						JSONPropertyNullOrAbsent("data"),
						m.JSONOptProperty("data").Should(m.JSONEqual(params.Data)),
						// we use JSONOptProperty for "data" here because the SDK is allowed to omit a null value
					),
					h.IfElse(!params.MetricValue.IsDefined(),
						JSONPropertyNullOrAbsent("metricValue"),
						m.JSONProperty("metricValue").Should(m.JSONEqual(params.MetricValue)),
					),
				),
			))
		})
	}
}
