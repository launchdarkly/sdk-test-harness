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
	URL         string
	sdkKind     SDKKind
	credential  string
	initialData SDKData
	streams     *eventsource.Server
	debugLogger framework.Logger
	lock        sync.RWMutex
}

type eventImpl struct {
	name string
	data map[string]interface{}
}

const (
	allDataChannel = "all"
)

func NewStreamingService(
	credential string,
	initialData SDKData,
	debugLogger framework.Logger,
) *StreamingService {
	streams := eventsource.NewServer()
	streams.ReplayAll = true
	streams.Logger = eventSourceDebugLogger{debugLogger}

	s := &StreamingService{
		sdkKind:     initialData.SDKKind(),
		credential:  credential,
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
		authKey := r.Header.Get("Authorization")
		if authKey != s.credential {
			w.WriteHeader(http.StatusUnauthorized)
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
	s.logEvent(event)
	s.streams.Publish([]string{allDataChannel}, event)
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

	return eventImpl{
		name: "put",
		data: map[string]interface{}{
			"data": json.RawMessage(data),
		},
	}
}

func (s *StreamingService) PushUpdate(namespace, key string, data json.RawMessage) {
	s.streams.Publish([]string{allDataChannel}, eventImpl{
		name: "patch",
		data: map[string]interface{}{
			"path": fmt.Sprintf("%s/%s", namespace, key),
			"data": data,
		},
	})
}

func (s *StreamingService) PushDelete(namespace, key string, version int) {
	s.streams.Publish([]string{allDataChannel}, eventImpl{
		name: "delete",
		data: map[string]interface{}{
			"path":    fmt.Sprintf("%s/%s", namespace, key),
			"version": version,
		},
	})
}

func (s *StreamingService) Replay(channel, id string) chan eventsource.Event {
	eventsCh := make(chan eventsource.Event, 1)
	e := s.makePutEvent()
	s.logEvent(e)
	eventsCh <- e
	close(eventsCh)
	return eventsCh
}

func (s *StreamingService) logEvent(e eventsource.Event) {
	s.debugLogger.Printf("sending %s event with data: %s", e.Event(), e.Data())
}

func (e eventImpl) Event() string { return e.name }
func (e eventImpl) Id() string    { return "" } //nolint:stylecheck
func (e eventImpl) Data() string {
	bytes, _ := json.Marshal(e.data)
	return string(bytes)
}
