package mockld

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"

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
	sdkKind                SDKKind
	ignoreDuplicatePayload bool
	hostTimeOverride       time.Time
	responseStatus         int
	responseStatusCount    int
	payloadIDsSeen         map[string]bool
	handler                http.Handler
	logger                 framework.Logger
	lock                   sync.Mutex
}

func NewEventsService(sdkKind SDKKind, logger framework.Logger, requireGzip bool) *EventsService {
	s := &EventsService{
		AnalyticsEventPayloads: make(chan Events, eventsChannelBufferSize),
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
		router.HandleFunc("/diagnostic", s.postDiagnosticEvent).Methods("POST")
	case RokuSDK:
		fallthrough
	case MobileSDK:
		router.HandleFunc("/mobile", s.postEvents).Methods("POST")
		router.HandleFunc("/mobile/events", s.postEvents).Methods("POST")
		router.HandleFunc("/mobile/events/bulk", s.postEvents).Methods("POST")
		router.HandleFunc("/mobile/events/diagnostic", s.postDiagnosticEvent).Methods("POST")
	case JSClientSDK:
		router.HandleFunc("/events/bulk/{env}", s.postEvents).Methods("POST")
		router.HandleFunc("/events/diagnostic/{env}", s.postDiagnosticEvent).Methods("POST")
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

// SetResponseStatus makes the events endpoint return the given HTTP status code for all subsequent
// analytics event posts, without recording the payload, until it is called again with a different
// value. A value of 0 (the default) restores normal behavior (accept with 202). Tests use this to
// exercise SDK behavior on delivery failures, such as the event-processor cooldown after an
// unexpected error, where the failure must persist for the duration of the cooldown window.
func (s *EventsService) SetResponseStatus(code int) {
	s.lock.Lock()
	s.responseStatus = code
	s.responseStatusCount = -1 // indefinite
	s.lock.Unlock()
}

// SetResponseStatusForNextPosts makes the events endpoint return the given HTTP status code for the
// next count analytics event posts (without recording their payloads), after which it automatically
// resumes normal behavior (accept with 202). Unlike SetResponseStatus, this needs no follow-up reset
// call, so the point at which the endpoint starts accepting again is deterministic rather than racing
// a test-side SetResponseStatus(0): the first count posts fail and every post after that succeeds,
// regardless of when the SDK happens to make them. Tests use it for failures that should affect only
// a bounded number of attempts, such as a single dropped or recoverable post.
//
// A count of zero or less is treated as "fail no posts" and restores normal behavior, rather than
// leaving an indefinite failure (which is SetResponseStatus's role) -- so "fail the next N" never
// becomes an accidental permanent failure for a degenerate N.
func (s *EventsService) SetResponseStatusForNextPosts(code int, count int) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if count <= 0 {
		s.responseStatus = 0
		s.responseStatusCount = 0
		return
	}
	s.responseStatus = code
	s.responseStatusCount = count
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
	responseStatus := s.responseStatus
	if responseStatus != 0 {
		// A bounded injected failure (see SetResponseStatusForNextPosts) consumes one of its remaining
		// posts and clears itself once exhausted; an indefinite one (count -1) is left untouched.
		if s.responseStatusCount > 0 {
			s.responseStatusCount--
			if s.responseStatusCount == 0 {
				s.responseStatus = 0
			}
		}
	} else if payloadID != "" {
		// Only record the payload ID for posts we actually accept, so an injected failure does not
		// poison dedup for a later retry that reuses the same payload ID.
		s.payloadIDsSeen[payloadID] = true
	}
	s.lock.Unlock()

	// If a response status has been injected (see SetResponseStatus / SetResponseStatusForNextPosts),
	// return it instead of accepting the payload, and do not record the payload. This lets tests
	// exercise SDK behavior on delivery failures, such as the event-processor cooldown after an
	// unexpected error. The endpoint still registers the connection, so connection-level assertions
	// continue to observe the attempt.
	if responseStatus != 0 {
		w.WriteHeader(responseStatus)
		s.logger.Printf("Returning injected HTTP status %d for event post (payload ID %q)", responseStatus, payloadID)
		return
	}

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
