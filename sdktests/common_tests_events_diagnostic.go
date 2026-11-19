package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

const diagnosticInitEventKind = "diagnostic-init"

// Wait used to confirm that no diagnostic event arrives on the analytics events route after the
// analytics payload.
const noMisroutedDiagnosticTimeout = 100 * time.Millisecond

// DiagnosticEvents verifies the routing of diagnostic events. With diagnostics enabled, the SDK
// sends a diagnostic-init event to the diagnostic events route. The SDK does not send a diagnostic
// event to the analytics events route. Gated on the diagnostic-events capability.
func (c CommonEventTests) DiagnosticEvents(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityDiagnosticEvents)

	t.Run("init event is sent to the diagnostic route only", func(t *ldtest.T) {
		dataSystem := NewSDKDataSystem(t, nil)
		events := NewSDKEventSink(t)
		eventsConfig := baseEventsConfig()
		eventsConfig.EnableDiagnostics = true
		// WithEventsConfig must precede the event sink: configurers apply in order, and the sink merges
		// the mock endpoint's base URI into the events config, which WithEventsConfig would overwrite.
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(
			WithEventsConfig(eventsConfig),
			dataSystem,
			events,
		)...)

		diagnostic := events.ExpectDiagnosticEvent(t, defaultEventTimeout)
		m.In(t).For("first diagnostic event").Assert(diagnostic, EventHasKind(diagnosticInitEventKind))

		// Make the SDK send an analytics payload. The events service records the misrouted diagnostic
		// events of a post before it delivers the analytics payload of that post.
		c.sendArbitraryEvent(t, client)
		client.FlushEvents(t)
		_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		events.ExpectNoMisroutedDiagnosticEvents(t, noMisroutedDiagnosticTimeout)
	})
}
