package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

// noMisroutedDiagnosticTimeout is how long the test waits after the analytics payload for a
// diagnostic event on the analytics events route.
const noMisroutedDiagnosticTimeout = 100 * time.Millisecond

// DiagnosticEvents verifies the routing of diagnostic events. With diagnostics enabled, the SDK
// must send a diagnostic-init event to the diagnostic events route. The test also fails if a
// diagnostic event arrives on the analytics events route before the analytics payload, or in the
// payload, or within noMisroutedDiagnosticTimeout after it. The test does not wait for the periodic
// diagnostic event, because its interval is not configurable and is many minutes long.
func (c CommonEventTests) DiagnosticEvents(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityDiagnosticEvents)

	t.Run("init event is sent to the diagnostic route only", func(t *ldtest.T) {
		dataSystem := NewSDKDataSystem(t, nil)
		events := NewSDKEventSink(t)
		eventsConfig := baseEventsConfig()
		eventsConfig.EnableDiagnostics = true
		// Configurers apply in order. WithEventsConfig replaces the events config, and the sink
		// adds the base URI of the mock endpoint to it. So WithEventsConfig comes first.
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(
			WithEventsConfig(eventsConfig),
			dataSystem,
			events,
		)...)

		diagnostic := events.ExpectDiagnosticEvent(t, defaultEventTimeout)
		m.In(t).For("first diagnostic event").Assert(diagnostic, IsDiagnosticInitEvent())

		// The events service records the misrouted diagnostic events of a post before it delivers
		// the analytics payload of that post.
		c.sendArbitraryEvent(t, client)
		client.FlushEvents(t)
		_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		events.ExpectNoMisroutedDiagnosticEvents(t, noMisroutedDiagnosticTimeout)
	})
}
