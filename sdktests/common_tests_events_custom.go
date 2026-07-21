package sdktests

import (
	"fmt"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/data"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

func (c CommonEventTests) CustomEvents(t *ldtest.T) {
	// These do not include detailed tests of the encoding of context attributes in custom events,
	// which are in common_tests_events_contexts.go.
	willInlineContext := t.Capabilities().Has(servicedef.CapabilityInlineContextAll) || c.isPHP

	t.Run("data and metricValue parameters", c.customEventsParameterizedTests)

	t.Run("basic properties", func(t *ldtest.T) {
		metricValue := 1.0

		customEventProperties := []string{
			"kind", "creationDate", "key", "data", "metricValue",
			h.IfElse(willInlineContext, "context", "contextKeys"),
		}
		if t.Capabilities().Has(servicedef.CapabilityClientSide) &&
			!t.Capabilities().Has(servicedef.CapabilityMobile) {
			// this is a JS-based client-side SDK, so custom events may have an additional property
			customEventProperties = append(customEventProperties, "url")
		}

		for _, contexts := range data.NewContextFactoriesForSingleAndMultiKind(c.contextFactory.Prefix()) {
			t.Run(contexts.Description(), func(t *ldtest.T) {
				context := contexts.NextUniqueContext()

				dataSource := NewSDKDataSource(t, nil)
				events := NewSDKEventSinkWithGzip(t, t.Capabilities().Has(servicedef.CapabilityEventGzip))
				client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSource, events)...)

				if c.isClientSide {
					client.SendIdentifyEvent(t, context)
					client.FlushEvents(t)
					_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				}

				client.SendCustomEvent(t, servicedef.CustomEventParams{
					EventKey:    "event-key",
					Context:     o.Some(context),
					Data:        ldvalue.Bool(true),
					MetricValue: o.Some(metricValue),
				})

				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

				expectedEvents := []m.Matcher{}
				if !c.isClientSide && !c.isPHP {
					expectedEvents = append(expectedEvents, IsIndexEvent())
				}
				expectedEvents = append(expectedEvents, m.AllOf(
					JSONPropertyKeysCanOnlyBe(customEventProperties...),
					IsCustomEvent(),
					h.IfElse(willInlineContext, HasContextObjectWithMatchingKeys(context), HasContextKeys(context)),
				))
				m.In(t).Assert(payload, m.ItemsInAnyOrder(expectedEvents...))
			})
		}
	})

	// Server-side SDKs inline the full context in custom events and redact anonymous context
	// attributes. Client-side SDKs use the current (identified) context for custom events and do
	// NOT redact anonymous contexts in custom events -- only in feature events. The client-side
	// behavior is covered separately below.
	if t.Capabilities().Has(servicedef.CapabilityAnonymousRedaction) &&
		t.Capabilities().Has(servicedef.CapabilityInlineContextAll) &&
		!c.isClientSide {
		t.Run("single-kind anonymous context redacts all attributes", func(t *ldtest.T) {
			anonymousFactory := data.NewContextFactory("anonymous", func(b *ldcontext.Builder) {
				b.Anonymous(true)
				b.Name("Example name")
				b.SetString("setup", "Why do programmers always confused Halloween and Christmas?")
				b.SetString("punchline", "Because OCT 31 = DEC 25")
			})
			anonymousContext := anonymousFactory.NextUniqueContext()

			dataSource := NewSDKDataSource(t, nil)
			events := NewSDKEventSinkWithGzip(t, t.Capabilities().Has(servicedef.CapabilityEventGzip))
			client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSource, events)...)
			client.SendCustomEvent(t, servicedef.CustomEventParams{
				EventKey: "event-key",
				Context:  o.Some(anonymousContext),
			})
			client.FlushEvents(t)

			expectedContext := ldcontext.NewBuilderFromContext(anonymousContext).
				SetValue("name", ldvalue.Null()).
				SetValue("setup", ldvalue.Null()).
				SetValue("punchline", ldvalue.Null()).
				Build()

			expectedEvents := []m.Matcher{}
			if !c.isClientSide && !c.isPHP {
				expectedEvents = append(expectedEvents, IsIndexEvent())
			}

			expectedContextMatcher := JSONMatchesEventContext(
				expectedContext,
				map[string][]string{"user": {"name", "setup", "punchline"}})
			expectedEvents = append(
				expectedEvents,
				m.AllOf(IsCustomEvent(), m.JSONProperty("context").Should(expectedContextMatcher)))

			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
			m.In(t).Assert(payload, m.ItemsInAnyOrder(expectedEvents...))
		})

		t.Run("multi-kind with anonymous context redacts attributes appropriately", func(t *ldtest.T) {
			userContextFactory := data.NewContextFactory("user", func(b *ldcontext.Builder) {
				b.Anonymous(true)
				b.Kind("user")
				b.Name("User name")
				b.SetString("setup", "Why do programmers always confused Halloween and Christmas?")
				b.SetString("punchline", "Because OCT 31 = DEC 25")
			})
			orgContextFactory := data.NewContextFactory("org", func(b *ldcontext.Builder) {
				b.Name("Org name")
				b.Kind("org")
				b.SetString("setup", "Why did the edge server go bankrupt?")
				b.SetString("punchline", "Because it ran out of cache")
			})

			userContext := userContextFactory.NextUniqueContext()
			orgContext := orgContextFactory.NextUniqueContext()
			multiContext := ldcontext.NewMultiBuilder().Add(userContext).Add(orgContext).Build()

			dataSource := NewSDKDataSource(t, nil)
			events := NewSDKEventSinkWithGzip(t, t.Capabilities().Has(servicedef.CapabilityEventGzip))
			client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSource, events)...)

			client.SendCustomEvent(t, servicedef.CustomEventParams{
				EventKey: "event-key",
				Context:  o.Some(multiContext),
			})
			client.FlushEvents(t)

			expectedUser := ldcontext.NewBuilderFromContext(userContext).
				SetValue("name", ldvalue.Null()).
				SetValue("setup", ldvalue.Null()).
				SetValue("punchline", ldvalue.Null()).
				Build()

			expectedMultiKind := ldcontext.NewMultiBuilder().Add(expectedUser).Add(orgContext).Build()

			expectedEvents := []m.Matcher{}
			if !c.isClientSide && !c.isPHP {
				expectedEvents = append(expectedEvents, IsIndexEvent())
			}

			expectedContextMatcher := JSONMatchesEventContext(
				expectedMultiKind,
				map[string][]string{"user": {"name", "setup", "punchline"}})
			expectedEvents = append(
				expectedEvents,
				m.AllOf(IsCustomEvent(), m.JSONProperty("context").Should(expectedContextMatcher)))

			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
			m.In(t).Assert(payload, m.ItemsInAnyOrder(expectedEvents...))
		})
	}

	// Client-side SDKs use the current (identified) context for custom events and do NOT redact
	// anonymous context attributes in custom events -- only feature events redact anonymous
	// contexts on the client. Establish the context via identify (discarding that identify event),
	// then verify the custom event carries the full, unredacted context.
	if t.Capabilities().Has(servicedef.CapabilityAnonymousRedaction) &&
		t.Capabilities().Has(servicedef.CapabilityInlineContextAll) &&
		c.isClientSide {
		t.Run("single-kind anonymous context is not redacted", func(t *ldtest.T) {
			anonymousFactory := data.NewContextFactory("anonymous", func(b *ldcontext.Builder) {
				b.Anonymous(true)
				b.Name("Example name")
				b.SetString("setup", "Why do programmers always confused Halloween and Christmas?")
				b.SetString("punchline", "Because OCT 31 = DEC 25")
			})
			anonymousContext := anonymousFactory.NextUniqueContext()

			dataSource := NewSDKDataSource(t, nil)
			events := NewSDKEventSinkWithGzip(t, t.Capabilities().Has(servicedef.CapabilityEventGzip))
			client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSource, events)...)

			client.SendIdentifyEvent(t, anonymousContext)
			client.FlushEvents(t)
			_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout) // discard the identify event

			client.SendCustomEvent(t, servicedef.CustomEventParams{
				EventKey: "event-key",
				Context:  o.Some(anonymousContext),
			})
			client.FlushEvents(t)

			// No redaction: the full anonymous context (including its attributes) is expected.
			expectedContextMatcher := JSONMatchesEventContext(anonymousContext, nil)
			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
			m.In(t).Assert(payload, m.ItemsInAnyOrder(
				m.AllOf(IsCustomEvent(), m.JSONProperty("context").Should(expectedContextMatcher)),
			))
		})

		t.Run("multi-kind with anonymous context is not redacted", func(t *ldtest.T) {
			userContextFactory := data.NewContextFactory("user", func(b *ldcontext.Builder) {
				b.Anonymous(true)
				b.Kind("user")
				b.Name("User name")
				b.SetString("setup", "Why do programmers always confused Halloween and Christmas?")
				b.SetString("punchline", "Because OCT 31 = DEC 25")
			})
			orgContextFactory := data.NewContextFactory("org", func(b *ldcontext.Builder) {
				b.Name("Org name")
				b.Kind("org")
				b.SetString("setup", "Why did the edge server go bankrupt?")
				b.SetString("punchline", "Because it ran out of cache")
			})

			userContext := userContextFactory.NextUniqueContext()
			orgContext := orgContextFactory.NextUniqueContext()
			multiContext := ldcontext.NewMultiBuilder().Add(userContext).Add(orgContext).Build()

			dataSource := NewSDKDataSource(t, nil)
			events := NewSDKEventSinkWithGzip(t, t.Capabilities().Has(servicedef.CapabilityEventGzip))
			client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSource, events)...)

			client.SendIdentifyEvent(t, multiContext)
			client.FlushEvents(t)
			_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout) // discard the identify event

			client.SendCustomEvent(t, servicedef.CustomEventParams{
				EventKey: "event-key",
				Context:  o.Some(multiContext),
			})
			client.FlushEvents(t)

			// No redaction: the full multi-kind context (including all attributes) is expected.
			expectedContextMatcher := JSONMatchesEventContext(multiContext, nil)
			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
			m.In(t).Assert(payload, m.ItemsInAnyOrder(
				m.AllOf(IsCustomEvent(), m.JSONProperty("context").Should(expectedContextMatcher)),
			))
		})
	}
}

func (c CommonEventTests) customEventsParameterizedTests(t *ldtest.T) {
	dataSource := NewSDKDataSource(t, nil)
	events := NewSDKEventSinkWithGzip(t, t.Capabilities().Has(servicedef.CapabilityEventGzip))
	client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSource, events)...)

	if c.isClientSide {
		// ignore the initial identify event
		client.FlushEvents(t)
		_ = events.ExpectAnalyticsEvents(t, time.Second)
	}

	// Generate many permutations of 1. data types that can be used for the data parameter, if any, and
	// 2. metric value parameter, if any.
	allParams := make([]servicedef.CustomEventParams, 0, (len(data.MakeStandardTestValues())+1)*4)
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

		for _, dataValue := range data.MakeStandardTestValues() {
			params := baseParams
			params.Data = dataValue
			params.Context = o.Some(c.contextFactory.NextUniqueContext())
			allParams = append(allParams, params)
		}

		// Add another case where the data parameter is null and we also set omitNullData. This is a
		// hint to the test service for SDKs that may have a different API for "no data" than "optional
		// data which may be null", to make sure we're covering both methods.
		params := baseParams
		params.OmitNullData = true
		params.Context = o.Some(c.contextFactory.NextUniqueContext())
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

			expectedEvents := []m.Matcher{}
			if !c.isClientSide && !c.isPHP {
				expectedEvents = append(expectedEvents, IsIndexEvent())
			}
			expectedEvents = append(expectedEvents,
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
			)
			m.In(t).Assert(payload, m.ItemsInAnyOrder(expectedEvents...))
		})
	}
}
