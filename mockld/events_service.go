package mockld

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
)

// Somewhat arbitrary buffer size for the channel that we use as a queue for received events. We
// don't want the HTTP handler to block if the test logic doesn't happen to be consuming events
// immediately.
const eventsChannelBufferSize = 100

// EventsService is a simulation of the LaunchDarkly event-recorder service, allowing tests to
// receive event data from an SDK. This is a low-level component that tests normally don't need
// to interact with directly; most tests use the sdktests.SDKEventSink facade.
type EventsService struct {
	AnalyticsEventPayloads chan Events
	sdkKind                SDKKind
	credential             string
	ignoreDuplicatePayload bool
	hostTimeOverride       time.Time
	payloadIDsSeen         map[string]bool
	logger                 framework.Logger
	lock                   sync.Mutex
}

func NewEventsService(sdkKind SDKKind, credential string, logger framework.Logger) *EventsService {
	return &EventsService{
		AnalyticsEventPayloads: make(chan Events, eventsChannelBufferSize),
		sdkKind:                sdkKind,
		credential:             credential,
		ignoreDuplicatePayload: true,
		payloadIDsSeen:         make(map[string]bool),
		logger:                 logger,
	}
}

func (s *EventsService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.logger.Printf("Received %s %s", r.Method, r.URL)
	switch r.URL.Path {
	case "/bulk":
		s.postEvents(w, r)
	case "/diagnostic":
		s.postDiagnosticEvent(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (s *EventsService) AwaitAnalyticsEventPayload(timeout time.Duration) (Events, bool) {
	ep := helpers.TryReceive(s.AnalyticsEventPayloads, timeout)
	return ep.Value(), ep.IsDefined()
}

func (s *EventsService) SetHostTimeOverride(t time.Time) {
	s.lock.Lock()
	s.hostTimeOverride = t
	s.lock.Unlock()
}

// SetIgnoreDuplicatePayload sets whether we should keep track of X-LaunchDarkly-Payload-Id header values
// we have seen at this endpoint and ignore any posts containing the same header value. This is true by
// default; tests would only set it to false if they want to verify that a failed post was retried.
//
// The rationale for this being the default behavior is that an SDK might, due to unpredictable network
// issues, think an event post had failed when it really succeeded, and retry the post. We don't want
// that to disrupt tests. The payload ID is always the same in the case of a retry for this very reason--
// it allows event-recorder to ignore accidental redundant posts. We trust that SDKs will generate
// reasonably unique payload IDs for posts that are not retries.
func (s *EventsService) SetIgnoreDuplicatePayload(ignore bool) {
	s.lock.Lock()
	s.ignoreDuplicatePayload = ignore
	s.lock.Unlock()
}

func (s *EventsService) postEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		s.logger.Printf("Unable to read request body")
		return
	}

	payloadID := r.Header.Get("X-LaunchDarkly-Payload-ID")

	s.lock.Lock()
	hostTime := s.hostTimeOverride
	ignoreDuplicatePayload := s.ignoreDuplicatePayload
	seenPayloadID := s.payloadIDsSeen[payloadID]
	if payloadID != "" {
		s.payloadIDsSeen[payloadID] = true
	}
	s.lock.Unlock()

	if !hostTime.IsZero() {
		w.Header().Set("Date", hostTime.UTC().Format(http.TimeFormat))
	}

	if ignoreDuplicatePayload && payloadID != "" && seenPayloadID {
		w.WriteHeader(http.StatusAccepted)
		s.logger.Printf("Received & discarded duplicate payload ID %q: %s", payloadID, string(data))
		return
	}

	var events []Event
	if err := json.Unmarshal(data, &events); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		s.logger.Printf("Received bad event data (%s): %s", err, string(data))
		return
	}
	w.WriteHeader(http.StatusAccepted)
	s.logger.Printf("Received %d events", len(events))
	for _, e := range events {
		s.logger.Printf("    %s", e.JSONString())
	}
	s.AnalyticsEventPayloads <- events
}

func (s *EventsService) postDiagnosticEvent(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	w.WriteHeader(http.StatusAccepted)
}
