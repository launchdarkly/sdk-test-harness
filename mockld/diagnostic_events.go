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

// AwaitMisrouted waits for a diagnostic event that the SDK posted to the analytics events route. It
// returns false if no such event arrives before the timeout.
func (d *DiagnosticEvents) AwaitMisrouted(timeout time.Duration) (Event, bool) {
	e := helpers.TryReceive(d.Misrouted, timeout)
	return e.Value(), e.IsDefined()
}

// record puts the event into the channel. If the channel is full, it discards the event, so that the
// HTTP handler does not block. Most tests never read the diagnostic channels.
func (d *DiagnosticEvents) record(ch chan Event, e Event, description string) {
	d.logger.Printf("Received %s: %s", description, e.JSONString())
	if !helpers.NonBlockingSend(ch, e) {
		d.logger.Printf("Discarded %s because the channel was full", description)
	}
}

// recordMisrouted puts each diagnostic event from the given events into Misrouted. It ignores
// analytics events.
func (d *DiagnosticEvents) recordMisrouted(events ...Event) {
	for _, e := range events {
		if e.IsDiagnostic() {
			d.record(d.Misrouted, e, "a diagnostic event on the analytics events route")
		}
	}
}

// postDiagnosticEvent always returns 202. It records the body in Payloads only if the body is a JSON
// object.
func (d *DiagnosticEvents) postDiagnosticEvent(w http.ResponseWriter, r *http.Request) {
	// Read the body before the response status. After a final status, net/http does not send
	// "100 Continue", and a client that sent "Expect: 100-continue" waits for its own timeout.
	data, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	w.WriteHeader(http.StatusAccepted)
	if err != nil {
		d.logger.Printf("Unable to read diagnostic request body")
		return
	}
	var event Event
	if err := json.Unmarshal(data, &event); err != nil || event.AsValue().Type() != ldvalue.ObjectType {
		d.logger.Printf("Received bad diagnostic event data: %s", string(data))
		return
	}
	d.record(d.Payloads, event, "diagnostic event")
}
