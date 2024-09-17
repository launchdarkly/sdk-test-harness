package mockld

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/launchdarkly/sdk-test-harness/v2/framework"

	"github.com/launchdarkly/eventsource"

	"github.com/gorilla/mux"
)

const (
	StreamingPathServerSide         = "/all"
	StreamingPathMobileGet          = "/meval/{context}"
	StreamingPathMobileReport       = "/meval"
	StreamingPathRokuHandshake      = "/handshake"
	StreamingPathRokuEvaluate       = "/mevalalternate"
	StreamingPathJSClientGet        = "/eval/{env}/{context}"
	StreamingPathJSClientReport     = "/eval/{env}"
	StreamingPathContextBase64Param = "{context}"
	StreamingPathEnvIDParam         = "{env}"
)

const errClientSideStreamCanOnlyUseFlags = `A client-side test attempted to reference a namespace other than` +
	` "flags" in the mock streaming service.` +
	` This is a test logic error, since client-side streams have nowhere to put any data other than flag data.`

type eventSourceDebugLogger struct {
	logger framework.Logger
}

func (l eventSourceDebugLogger) Println(args ...interface{}) {
	l.logger.Printf("%s", fmt.Sprintln(args...))
}

func (l eventSourceDebugLogger) Printf(fmt string, args ...interface{}) {
	l.logger.Printf(fmt, args)
}

type StreamingService struct {
	sdkKind      SDKKind
	initialData  SDKData
	streams      *eventsource.Server
	queuedEvents []eventsource.Event
	started      bool
	handler      http.Handler
	debugLogger  framework.Logger
	lock         sync.RWMutex
}

type eventImpl struct {
	name string
	data interface{}
}

const (
	allDataChannel = "all"
)

func NewStreamingService(
	initialData SDKData,
	sdkKind SDKKind,
	debugLogger framework.Logger,
) *StreamingService {
	streams := eventsource.NewServer()
	streams.ReplayAll = true
	streams.Logger = eventSourceDebugLogger{debugLogger}

	s := &StreamingService{
		sdkKind:     sdkKind,
		initialData: initialData,
		streams:     streams,
		debugLogger: debugLogger,
	}

	streamHandler := streams.Handler(allDataChannel)
	router := mux.NewRouter()
	switch sdkKind {
	case ServerSideSDK:
		router.HandleFunc(StreamingPathServerSide, streamHandler).Methods("GET")
	case RokuSDK:
		rokuHandler := RokuServer{}

		router.Path(StreamingPathRokuHandshake).Methods("POST").HandlerFunc(rokuHandler.ServeHandshake)
		router.Path(StreamingPathRokuEvaluate).Methods("POST").Handler(rokuHandler.Wrap(s))
		fallthrough
	case MobileSDK:
		router.HandleFunc(StreamingPathMobileGet, streamHandler).Methods("GET")
		router.HandleFunc(StreamingPathMobileReport, streamHandler).Methods("REPORT")
	case JSClientSDK:
		router.HandleFunc(StreamingPathJSClientGet, streamHandler).Methods("GET")
		router.HandleFunc(StreamingPathJSClientReport, streamHandler).Methods("REPORT")
	}
	s.handler = router

	streams.Register(allDataChannel, s)

	return s
}

func (s *StreamingService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

func (s *StreamingService) SetInitialData(data SDKData) {
	s.lock.Lock()
	s.initialData = data
	s.lock.Unlock()
}

func (s *StreamingService) RefreshAll() {
	events := s.makeXferFull()
	for _, event := range events {
		s.logEvent(event)
		s.streams.Publish([]string{allDataChannel}, event)
	}
}

func (s *StreamingService) makeXferFull() []eventsource.Event {
	s.lock.RLock()

	fdv2SdkData, ok := s.initialData.(FDv2SDKData)
	if !ok {
		s.debugLogger.Println("poller cannot handle non-fdv2 sdk data at this time")
		return nil
	}

	// QUESTION: How dynamic do we need to bother making this?
	serverIntent := framework.ServerIntent{
		Payloads: []framework.Payload{
			{
				ID:     "payloadID",
				Target: 1,
				Code:   "xfer-full",
				Reason: "payload-missing",
			},
		},
	}

	// QUESTION: How dynamic do we need to bother making this?
	payloadTransferred := framework.PayloadTransferred{
		State:   "state", // TODO: Need to replace this with a valid state value
		Version: 1,
	}

	events := make([]eventsource.Event, 0, len(fdv2SdkData)+2)
	events = append(events, eventImpl{
		name: "server-intent",
		data: serverIntent,
	})

	for _, obj := range fdv2SdkData {
		events = append(events, eventImpl{
			name: "put-object",
			data: obj,
		})
	}

	events = append(events, eventImpl{
		name: "payload-transferred",
		data: payloadTransferred,
	})

	s.lock.RUnlock()

	return events
}

// PushEvent sends an SSE event to all clients that are currently connected to the stream-- or, if no client
// has connected yet, queues the event so that it will be sent (after the initial data) to the
// first client that connects. (The latter is necessary to avoid race conditions, since even after
// a connection is received on the stream endpoint, it is hard for the test logic to know when the
// HTTP handler has actually created a stream subscription for that connection. If we called
// the eventsource Publish method before a subscription existed, the event would be lost.)
func (s *StreamingService) PushEvent(eventName string, eventData interface{}) {
	event := eventImpl{
		name: eventName,
		data: eventData,
	}

	s.lock.Lock()
	alreadyStarted := s.started
	if !alreadyStarted {
		s.queuedEvents = append(s.queuedEvents, event)
	}
	s.lock.Unlock()

	if alreadyStarted {
		s.logEvent(event)
		s.streams.Publish([]string{allDataChannel}, event)
	} else {
		s.debugLogger.Printf("Will send %q event after connection has started", eventName)
	}
}

func (s *StreamingService) PushUpdate(namespace, key string, version int, data json.RawMessage) {
	var eventData interface{}
	if s.sdkKind.IsServerSide() {
		eventData = framework.BaseObject{
			Version: version,
			Kind:    namespace,
			Key:     key,
			Object:  data,
		}
	} else {
		if namespace != "flag" {
			panic(errClientSideStreamCanOnlyUseFlags)
		}
		eventData = data
	}
	s.PushEvent("put-object", eventData)
}

func (s *StreamingService) PushPayloadTransferred(state string, version int) {
	eventData := framework.PayloadTransferred{
		State:   state,
		Version: version,
	}
	s.PushEvent("payload-transferred", eventData)
}

func (s *StreamingService) PushDelete(namespace, key string, version int) {
	var eventData interface{}
	if s.sdkKind.IsServerSide() {
		eventData = framework.BaseObject{
			Version: version,
			Kind:    namespace,
			Key:     key,
		}
	} else {
		if namespace != "flag" {
			panic(errClientSideStreamCanOnlyUseFlags)
		}

		// TODO: Update this to match whatever the client fdv2 format should look like
		eventData = map[string]interface{}{
			"key":     key,
			"version": version,
		}
	}
	s.PushEvent("delete-object", eventData)
}

func (s *StreamingService) Replay(channel, id string) chan eventsource.Event {
	events := s.makeXferFull()

	// The use of a channel here is just part of how the eventsource server API works-- the Replay
	// method is expected to return a channel, which could be either pre-populated or pushed to
	// by another goroutine. In this case we're just pre-populating it with the same initial data
	// that we provide to every incoming connection, plus any events that were queued by test logic
	// before the connection actually started.

	s.lock.Lock()
	queued := s.queuedEvents
	eventsCh := make(chan eventsource.Event, len(queued)+len(events))
	if !s.started {
		s.started = true
		s.queuedEvents = nil
	}
	s.lock.Unlock()

	for _, event := range events {
		s.logEvent(event)
		eventsCh <- event
	}

	for _, qe := range queued {
		s.logEvent(qe)
		eventsCh <- qe
	}

	close(eventsCh)
	return eventsCh
}

func (s *StreamingService) logEvent(e eventsource.Event) {
	s.debugLogger.Printf("Sending %s event with data: %s", e.Event(), e.Data())
}

func (e eventImpl) Event() string { return e.name }
func (e eventImpl) Id() string    { return "" } //nolint:stylecheck
func (e eventImpl) Data() string {
	if raw, ok := e.data.(json.RawMessage); ok {
		return string(raw) // this allows us to pass malformed data that json.Marshal wouldn't allow
	}
	bytes, _ := json.Marshal(e.data)
	return string(bytes)
}
