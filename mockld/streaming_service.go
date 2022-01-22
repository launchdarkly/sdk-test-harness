package mockld

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/launchdarkly/sdk-test-harness/framework"

	"github.com/launchdarkly/eventsource"
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
	streams.Register(allDataChannel, s)

	return s
}

func (s *StreamingService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/all" && s.sdkKind == ServerSideSDK:
		if r.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		s.streams.Handler(allDataChannel)(w, r)
		s.debugLogger.Printf("End of stream request")
	default:
		w.WriteHeader(http.StatusNotFound)
	}
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
	return eventImpl{
		name: "put",
		data: map[string]interface{}{
			"data": json.RawMessage(data),
		},
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
	s.debugLogger.Printf("sending %s event with data: %s", e.Event(), e.Data())
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
