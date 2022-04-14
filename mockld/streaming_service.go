package mockld

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/launchdarkly/sdk-test-harness/framework"

	"github.com/launchdarkly/eventsource"

	"github.com/gorilla/mux"
)

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
	sdkKind     SDKKind
	initialData SDKData
	streams     *eventsource.Server
	handler     http.Handler
	debugLogger framework.Logger
	lock        sync.RWMutex
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
	debugLogger framework.Logger,
) *StreamingService {
	streams := eventsource.NewServer()
	streams.ReplayAll = true
	streams.Logger = eventSourceDebugLogger{debugLogger}

	s := &StreamingService{
		sdkKind:     initialData.SDKKind(),
		initialData: initialData,
		streams:     streams,
		debugLogger: debugLogger,
	}

	streamHandler := streams.Handler(allDataChannel)
	router := mux.NewRouter()
	switch initialData.SDKKind() {
	case ServerSideSDK:
		router.HandleFunc("/all", streamHandler).Methods("GET")
	case MobileSDK:
		router.HandleFunc("/meval/{user}", streamHandler).Methods("GET")
		router.HandleFunc("/meval", streamHandler).Methods("REPORT")
	case JSClientSDK:
		router.HandleFunc("/eval/{env}/{user}", streamHandler).Methods("GET")
		router.HandleFunc("/eval/{env}", streamHandler).Methods("REPORT")
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
	event := s.makePutEvent()
	if event != nil {
		s.logEvent(event)
		s.streams.Publish([]string{allDataChannel}, event)
	}
}

func (s *StreamingService) makePutEvent() eventsource.Event {
	s.lock.RLock()
	var data []byte
	if s.initialData == nil {
		data = []byte("{}")
	} else {
		data = s.initialData.Serialize()
	}
	s.lock.RUnlock()

	if data == nil {
		return nil
	}
	var eventData interface{} = json.RawMessage(data)
	if s.sdkKind == ServerSideSDK {
		// the schema of this message is slightly different for server-side vs. client-side
		eventData = map[string]interface{}{
			"data": eventData,
		}
	}

	return eventImpl{
		name: "put",
		data: eventData,
	}
}

func (s *StreamingService) PushEvent(eventName string, eventData interface{}) {
	event := eventImpl{
		name: eventName,
		data: eventData,
	}
	s.logEvent(event)
	s.streams.Publish([]string{allDataChannel}, event)
}

func (s *StreamingService) PushUpdate(namespace, key string, data json.RawMessage) {
	s.PushEvent("patch",
		map[string]interface{}{
			"path": fmt.Sprintf("/%s/%s", namespace, key),
			"data": data,
		})
}

func (s *StreamingService) PushDelete(namespace, key string, version int) {
	s.PushEvent("delete",
		map[string]interface{}{
			"path":    fmt.Sprintf("/%s/%s", namespace, key),
			"version": version,
		})
}

func (s *StreamingService) Replay(channel, id string) chan eventsource.Event {
	e := s.makePutEvent()

	// The use of a channel here is just part of how the eventsource server API works-- the Replay
	// method is expected to return a channel, which could be either pre-populated or pushed to
	// by another goroutine. In this case we're just pre-populating it with the same initial data
	// that we provide to every incoming connection.
	eventsCh := make(chan eventsource.Event, 1)
	if e != nil {
		s.logEvent(e)
		eventsCh <- e
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
