package mockld

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework"
)

// EventsService is a simulation of the LaunchDarkly event-recorder service, allowing tests to
// receive event data from an SDK.
type EventsService struct {
	AnalyticsEventPayloads chan Events
	sdkKind                SDKKind
	credential             string
	logger                 framework.Logger
}

func NewEventsService(sdkKind SDKKind, credential string, logger framework.Logger) *EventsService {
	return &EventsService{
		AnalyticsEventPayloads: make(chan Events, 100),
		sdkKind:                sdkKind,
		credential:             credential,
		logger:                 logger,
	}
}

func (s *EventsService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.logger.Printf("received %s %s", r.Method, r.URL)
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
	select {
	case ep := <-s.AnalyticsEventPayloads:
		return ep, true
	case <-time.After(timeout):
		return nil, false
	}
}

func (s *EventsService) postEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		s.logger.Printf("unable to read request body")
		return
	}
	var events []Event
	if err := json.Unmarshal(data, &events); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		s.logger.Printf("received bad event data (%s): %s", err, string(data))
		return
	}
	w.WriteHeader(http.StatusAccepted)
	s.logger.Printf("received %d events", len(events))
	for _, e := range events {
		s.logger.Printf("    %s", e.JSONString())
	}
	s.AnalyticsEventPayloads <- events
}

func (s *EventsService) postDiagnosticEvent(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	w.WriteHeader(http.StatusAccepted)
}
