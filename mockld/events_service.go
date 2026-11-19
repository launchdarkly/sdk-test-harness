package mockld

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v3/framework"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/helpers"

	"github.com/gorilla/mux"
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
	// Diagnostics records the diagnostic events that the SDK sends to this service.
	Diagnostics            *DiagnosticEvents
	sdkKind                SDKKind
	ignoreDuplicatePayload bool
	hostTimeOverride       time.Time
	payloadIDsSeen         map[string]bool
	handler                http.Handler
	logger                 framework.Logger
	lock                   sync.Mutex
}

func NewEventsService(sdkKind SDKKind, logger framework.Logger, requireGzip bool) *EventsService {
	s := &EventsService{
		AnalyticsEventPayloads: make(chan Events, eventsChannelBufferSize),
		Diagnostics:            newDiagnosticEvents(logger),
		sdkKind:                sdkKind,
		ignoreDuplicatePayload: true,
		payloadIDsSeen:         make(map[string]bool),
		logger:                 logger,
	}

	router := mux.NewRouter()
	if requireGzip {
		router.Use(gzipMiddleware)
	}

	switch sdkKind {
	case ServerSideSDK, PHPSDK:
		router.HandleFunc("/bulk", s.postEvents).Methods("POST")
		router.HandleFunc("/diagnostic", s.Diagnostics.postDiagnosticEvent).Methods("POST")
	case RokuSDK:
		fallthrough
	case MobileSDK:
		router.HandleFunc("/mobile", s.postEvents).Methods("POST")
		router.HandleFunc("/mobile/events", s.postEvents).Methods("POST")
		router.HandleFunc("/mobile/events/bulk", s.postEvents).Methods("POST")
		router.HandleFunc("/mobile/events/diagnostic", s.Diagnostics.postDiagnosticEvent).Methods("POST")
	case JSClientSDK:
		router.HandleFunc("/events/bulk/{env}", s.postEvents).Methods("POST")
		router.HandleFunc("/events/diagnostic/{env}", s.Diagnostics.postDiagnosticEvent).Methods("POST")
	}
	s.handler = router

	return s
}

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") {
			gr, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "Failed to read gzipped request body", http.StatusBadRequest)
				return
			}
			r.Body = gr
			next.ServeHTTP(w, r)
		} else {
			http.Error(w, "Failed to provide a gzipped payload", http.StatusBadRequest)
		}
	})
}

func (s *EventsService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.logger.Printf("Received %s %s", r.Method, r.URL)
	s.handler.ServeHTTP(w, r)
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
	data, err := io.ReadAll(r.Body)
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

	// Look for misrouted diagnostic events before the duplicate check, because a post with a
	// payload ID that was already seen can also hold a diagnostic event. A diagnostic event is one
	// JSON object, not an array, so a body that does not decode as a list is decoded as one event.
	var events []Event
	decodeErr := json.Unmarshal(data, &events)
	if decodeErr == nil {
		// This is before the delivery of the payload. A test that has the payload can then find all
		// the misrouted events of the same post without a wait.
		s.Diagnostics.recordMisrouted(events...)
	} else {
		var single Event
		if json.Unmarshal(data, &single) == nil {
			s.Diagnostics.recordMisrouted(single)
		}
	}

	if ignoreDuplicatePayload && payloadID != "" && seenPayloadID {
		w.WriteHeader(http.StatusAccepted)
		s.logger.Printf("Received & discarded duplicate payload ID %q: %s", payloadID, string(data))
		return
	}

	if decodeErr != nil {
		w.WriteHeader(http.StatusBadRequest)
		s.logger.Printf("Received bad event data (%s): %s", decodeErr, string(data))
		return
	}
	w.WriteHeader(http.StatusAccepted)
	s.logger.Printf("Received %d events", len(events))
	for _, e := range events {
		s.logger.Printf("    %s", e.JSONString())
	}
	s.AnalyticsEventPayloads <- events
}
