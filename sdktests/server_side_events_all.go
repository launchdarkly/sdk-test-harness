package sdktests

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

func doServerSideEventTests(t *ldtest.T) {
	t.Run("requests", doServerSideEventRequestTests)
	t.Run("gzip", doServerSideGzipEventRequestTests)
	t.Run("summary events", doServerSideSummaryEventTests)
	t.Run("feature events", doServerSideFeatureEventTests)
	t.Run("debug events", doServerSideDebugEventTests)
	t.Run("feature prerequisite events", doServerSideFeaturePrerequisiteEventTests)
	t.Run("experimentation", doServerSideExperimentationEventTests)
	t.Run("identify events", doServerSideIdentifyEventTests)
	t.Run("custom events", doServerSideCustomEventTests)
	t.Run("index events", doServerSideIndexEventTests)
	t.Run("context properties", doServerSideEventContextTests)
	t.Run("event capacity", doServerSideEventBufferTests)
	t.Run("disabling", doServerSideEventDisableTest)
}

func doServerSideEventRequestTests(t *ldtest.T) {
	sdkKey := "my-sdk-key"

	eventTests := NewCommonEventTests(t, "doServerSideEventRequestTests",
		WithConfig(servicedef.SDKConfigParams{
			Credential: sdkKey,
		}))

	eventTests.RequestMethodAndHeaders(t, sdkKey, m.AllOf(
		Header("X-LaunchDarkly-Event-Schema").Should(m.Equal(currentEventSchema)),
		Header("X-LaunchDarkly-Payload-Id").Should(m.Not(m.Equal(""))),
		Header("Content-Type").Should(m.StringContains("application/json")),
	))

	eventTests.RequestURLPath(t, m.Equal("/bulk"))

	eventTests.UniquePayloadIDs(t)
}

func doServerSideGzipEventRequestTests(t *ldtest.T) {
	if !t.Capabilities().Has(servicedef.CapabilityEventGzip) {
		return
	}

	dataBuilder := mockld.NewServerSDKDataBuilder()
	dataSource := NewSDKDataSource(t, dataBuilder.Build())

	for _, enableGzip := range []bool{true, false} {
		if !enableGzip && !t.Capabilities().Has(servicedef.CapabilityOptionalEventGzip) {
			continue
		}

		desc := "enabled"
		if !enableGzip {
			desc = "disabled"
		}

		t.Run(desc, func(t *ldtest.T) {
			events := NewSDKEventSinkWithGzip(t, enableGzip)

			eventsConfig := baseEventsConfig()
			eventsConfig.EnableGzip = o.Some(enableGzip)
			eventsConfig.BaseURI = events.eventsEndpoint.BaseURL()

			client := NewSDKClient(t, dataSource, events, WithEventsConfig(eventsConfig))
			client.SendIdentifyEvent(t, ldcontext.New("user"))
			client.FlushEvents(t)
			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

			m.In(t).Assert(payload, m.ItemsInAnyOrder(
				IsIdentifyEvent(),
			))
		})
	}
}

func doServerSideIdentifyEventTests(t *ldtest.T) {
	NewCommonEventTests(t, "doServerSideIdentifyEventTests").
		IdentifyEvents(t)
}

func doServerSideCustomEventTests(t *ldtest.T) {
	NewCommonEventTests(t, "doServerSideCustomEventTests").
		CustomEvents(t)
}

func doServerSideEventContextTests(t *ldtest.T) {
	NewCommonEventTests(t, "doServerSideEventContextTests").
		EventContexts(t)
}

func doServerSideEventBufferTests(t *ldtest.T) {
	NewCommonEventTests(t, "doServerSideEventCapacityTests").
		BufferBehavior(t)
}

func doServerSideEventDisableTest(t *ldtest.T) {
	NewCommonEventTests(t, "doServerSideEventDisableTest").
		DisablingEvents(t)
}
