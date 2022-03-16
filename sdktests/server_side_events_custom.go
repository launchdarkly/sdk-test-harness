package sdktests

import (
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldvalue"
)

func doServerSideCustomEventTests(t *ldtest.T) {
	// These do not include detailed tests of the encoding of context attributes in custom events,
	// which are in server_side_events_contexts.go.

	t.Run("data and metricValue parameters", doServerSideParameterizedCustomEventTests)

	t.Run("basic properties", func(t *ldtest.T) {
		metricValue := 1.0
		for _, contexts := range data.NewContextFactoriesForSingleAndMultiKind("doServerSideCustomEventTests") {
			t.Run(contexts.Description(), func(t *ldtest.T) {
				context := contexts.NextUniqueContext()

				dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
				events := NewSDKEventSink(t)
				client := NewSDKClient(t, dataSource, events)

				client.SendCustomEvent(t, servicedef.CustomEventParams{
					EventKey:    "event-key",
					Context:     context,
					Data:        ldvalue.Bool(true),
					MetricValue: &metricValue,
				})

				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

				m.In(t).Assert(payload, m.ItemsInAnyOrder(
					IsIndexEvent(),
					m.AllOf(
						JSONPropertyKeysCanOnlyBe("kind", "creationDate", "key", "contextKeys", "data", "metricValue"),
						IsCustomEvent(),
						HasContextKeys(context),
					),
				))
			})
		}
	})
}

func doServerSideParameterizedCustomEventTests(t *ldtest.T) {
	dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSource, events)

	contexts := data.NewContextFactory("doServerSideParameterizedCustomEventTests")

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
			params.Context = contexts.NextUniqueContext()
			allParams = append(allParams, params)
		}

		// Add another case where the data parameter is null and we also set omitNullData. This is a
		// hint to the test service for SDKs that may have a different API for "no data" than "optional
		// data which may be null", to make sure we're covering both methods.
		params := baseParams
		params.OmitNullData = true
		params.Context = contexts.NextUniqueContext()
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
			m.In(t).Assert(payload, m.ItemsInAnyOrder(
				IsIndexEvent(), // we don't care about the index event
				m.AllOf(
					IsCustomEventForEventKey(params.EventKey),
					conditionalMatcher(params.OmitNullData && params.Data.IsNull(),
						JSONPropertyNullOrAbsent("data"),
						m.JSONOptProperty("data").Should(m.JSONEqual(params.Data)),
						// we use JSONOptProperty for "data" here because the SDK is allowed to omit a null value
					),
					conditionalMatcher(params.MetricValue == nil,
						JSONPropertyNullOrAbsent("metricValue"),
						m.JSONProperty("metricValue").Should(m.JSONEqual(params.MetricValue)),
					),
				),
			))
		})
	}
}
