package mockld

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v3/framework"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/helpers"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

// DiagnosticEvents records the diagnostic events that an SDK sends to an EventsService. It records
// the events from the diagnostic events route. It also records the diagnostic events that arrive on
// the analytics events route, because a correct SDK never sends them there.
type DiagnosticEvents struct {
	// Payloads receives the JSON object from each post to the diagnostic events route.
	Payloads chan Event
	// Misrouted receives each diagnostic event from a post to the analytics events route.
	Misrouted chan Event
	logger    framework.Logger
}

func newDiagnosticEvents(logger framework.Logger) *DiagnosticEvents {
	return &DiagnosticEvents{
		Payloads:  make(chan Event, eventsChannelBufferSize),
		Misrouted: make(chan Event, eventsChannelBufferSize),
		logger:    logger,
	}
}

// AwaitPayload waits for a diagnostic event that the SDK posted to the diagnostic events route. It
// returns false if no such event arrives before the timeout.
func (d *DiagnosticEvents) AwaitPayload(timeout time.Duration) (Event, bool) {
	e := helpers.TryReceive(d.Payloads, timeout)
	return e.Value(), e.IsDefined()
}

// AwaitMisrouted waits for a diagnostic event that the SDK posted to the analytics events route. It
// returns false if no such event arrives before the timeout.
func (d *DiagnosticEvents) AwaitMisrouted(timeout time.Duration) (Event, bool) {
	e := helpers.TryReceive(d.Misrouted, timeout)
	return e.Value(), e.IsDefined()
}

// recordMisrouted puts each diagnostic event from the given events into Misrouted. It ignores
// analytics events. It does not block the HTTP handler: if the channel is full, it discards the
// event.
func (d *DiagnosticEvents) recordMisrouted(events ...Event) {
	for _, e := range events {
		if !e.IsDiagnostic() {
			continue
		}
		d.logger.Printf("Received a diagnostic event on the analytics events route: %s", e.JSONString())
		if !helpers.NonBlockingSend(d.Misrouted, e) {
			d.logger.Printf("Discarded a misrouted diagnostic event because the channel was full")
		}
	}
}

// postDiagnosticEvent always returns 202. It records the body in Payloads only if the body is a JSON
// object. It does not block the HTTP handler: if the channel is full, it discards the event.
func (d *DiagnosticEvents) postDiagnosticEvent(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	w.WriteHeader(http.StatusAccepted)

	data, err := io.ReadAll(r.Body)
	if err != nil {
		d.logger.Printf("Unable to read diagnostic request body")
		return
	}
	var event Event
	if err := json.Unmarshal(data, &event); err != nil || event.AsValue().Type() != ldvalue.ObjectType {
		d.logger.Printf("Received bad diagnostic event data: %s", string(data))
		return
	}
	d.logger.Printf("Received diagnostic event: %s", event.JSONString())
	if !helpers.NonBlockingSend(d.Payloads, event) {
		d.logger.Printf("Discarded a diagnostic event because the channel was full")
	}
}
