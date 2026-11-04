package mockld

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/launchdarkly/sdk-test-harness/v3/framework"

	"github.com/launchdarkly/eventsource"
	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"

	"github.com/gorilla/mux"
)

const (
	StreamingPathServerSide         = "/sdk/stream"
	StreamingPathRokuHandshake      = "/handshake"
	StreamingPathRokuEvaluate       = "/mevalalternate"
	StreamingPathMobileGet          = "/meval/{context}"
	StreamingPathMobileReport       = "/meval"
	StreamingPathFDv2ClientGet      = "/sdk/stream/eval/{context}"
	StreamingPathFDv2ClientPost     = "/sdk/stream/eval"
	StreamingPathContextBase64Param = "{context}"

	StreamingPathFDv1ServerSide = "/all"
	StreamingPathJSClientGet    = "/eval/{env}/{context}"
	StreamingPathJSClientReport = "/eval/{env}"
	StreamingPathEnvIDParam     = "{env}"
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
	protocol    DataProtocol
	sdkKind     SDKKind
	initialData SDKData
	// payloadVersion is the FDv2 payload version that the protocol-neutral push methods sent last. The
	// initial payload has version 1.
	payloadVersion int
	streams        *eventsource.Server
	queuedEvents   []eventsource.Event
	started        bool
	handler        http.Handler
	debugLogger    framework.Logger
	lock           sync.RWMutex
}

type eventImpl struct {
	event string
	data  interface{}
}

const (
	allDataChannel = "all"

	// changedPayloadState is the FDv2 payload state that the protocol-neutral push methods send.
	changedPayloadState = "updated"
)

// NewStreamingService creates a mock streaming service that uses the FDv2 protocol.
func NewStreamingService(
	initialData SDKData,
	sdkKind SDKKind,
	debugLogger framework.Logger,
) *StreamingService {
	return NewStreamingServiceForProtocol(FDv2, initialData, sdkKind, debugLogger)
}

// NewStreamingServiceForProtocol creates a mock streaming service that uses the specified protocol.
func NewStreamingServiceForProtocol(
	protocol DataProtocol,
	initialData SDKData,
	sdkKind SDKKind,
	debugLogger framework.Logger,
) *StreamingService {
	streams := eventsource.NewServer()
	streams.ReplayAll = true
	streams.Logger = eventSourceDebugLogger{debugLogger}

	s := &StreamingService{
		protocol:       protocol,
		sdkKind:        sdkKind,
		initialData:    initialData,
		payloadVersion: 1,
		streams:        streams,
		debugLogger:    debugLogger,
	}

	streamHandler := streams.Handler(allDataChannel)
	router := mux.NewRouter()
	if protocol == FDv1 {
		switch sdkKind {
		case ServerSideSDK:
			router.HandleFunc(StreamingPathFDv1ServerSide, streamHandler).Methods("GET")
		case RokuSDK:
			rokuHandler := RokuServer{}
			router.Path(StreamingPathRokuHandshake).Methods("POST").HandlerFunc(rokuHandler.ServeHandshake)
			router.Path(StreamingPathRokuEvaluate).Methods("POST").Handler(rokuHandler.Wrap(s))
			router.HandleFunc(StreamingPathMobileGet, streamHandler).Methods("GET")
			router.HandleFunc(StreamingPathMobileReport, streamHandler).Methods("REPORT")
		case MobileSDK:
			router.HandleFunc(StreamingPathMobileGet, streamHandler).Methods("GET")
			router.HandleFunc(StreamingPathMobileReport, streamHandler).Methods("REPORT")
		case JSClientSDK:
			router.HandleFunc(StreamingPathJSClientGet, streamHandler).Methods("GET")
			router.HandleFunc(StreamingPathJSClientReport, streamHandler).Methods("REPORT")
		}
	} else {
		switch sdkKind {
		case ServerSideSDK:
			router.HandleFunc(StreamingPathServerSide, streamHandler).Methods("GET")
		case RokuSDK:
			rokuHandler := RokuServer{}
			router.Path(StreamingPathRokuHandshake).Methods("POST").HandlerFunc(rokuHandler.ServeHandshake)
			router.Path(StreamingPathRokuEvaluate).Methods("POST").Handler(rokuHandler.Wrap(s))
			router.HandleFunc(StreamingPathMobileGet, streamHandler).Methods("GET")
			router.HandleFunc(StreamingPathMobileReport, streamHandler).Methods("REPORT")
			router.HandleFunc(StreamingPathFDv2ClientGet, streamHandler).Methods("GET")
			router.HandleFunc(StreamingPathFDv2ClientPost, streamHandler).Methods("POST", "REPORT")
		case MobileSDK, JSClientSDK:
			router.HandleFunc(StreamingPathFDv2ClientGet, streamHandler).Methods("GET")
			router.HandleFunc(StreamingPathFDv2ClientPost, streamHandler).Methods("POST", "REPORT")
		}
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
	events := s.initialEvents()
	for _, event := range events {
		s.logEvent(event)
		s.streams.Publish([]string{allDataChannel}, event)
	}
}

// initialEvents returns the events that deliver the initial data to a new connection. There are no events
// if the data is deliberately unavailable.
func (s *StreamingService) initialEvents() []eventsource.Event {
	if s.protocol == FDv1 {
		s.lock.RLock()
		data := s.initialData
		s.lock.RUnlock()
		if putData := s.fdv1PutData(data); putData != nil {
			return []eventsource.Event{eventImpl{event: "put", data: putData}}
		}
		return nil
	}
	return s.makeXferFull()
}

// fdv1PutData returns the data of an FDv1 "put" event, or nil if the data is deliberately unavailable.
// The data of a server-side "put" event has the data set in its "data" property.
func (s *StreamingService) fdv1PutData(data SDKData) interface{} {
	payload := fdv1Payload(data, s.sdkKind)
	if payload == nil {
		return nil
	}
	if s.sdkKind.IsServerSide() {
		return map[string]interface{}{"data": json.RawMessage(payload)}
	}
	return json.RawMessage(payload)
}

// requireFDv2 panics if the service uses FDv1, because the named event exists only in FDv2.
func (s *StreamingService) requireFDv2(eventName string) {
	if s.protocol != FDv2 {
		panic(fmt.Sprintf("test logic error: an FDv1 stream cannot send the FDv2 %q event", eventName))
	}
}

func (s *StreamingService) makeXferFull() []eventsource.Event {
	s.lock.RLock()
	defer s.lock.RUnlock()

	fdv2SdkData, ok := s.initialData.(FDv2SDKData)
	if !ok {
		s.debugLogger.Println("streamer cannot handle non-fdv2 sdk data at this time")
		return nil
	}

	events := make([]eventsource.Event, 0, len(fdv2SdkData.events)+2)
	events = append(events, eventImpl{
		event: "server-intent",
		data: framework.ServerIntent{
			Payloads: []framework.Payload{
				{
					ID:     "payloadID",
					Target: 1,
					Code:   fdv2SdkData.intentCode,
					Reason: fdv2SdkData.intentReason,
				},
			},
		},
	})

	for _, obj := range fdv2SdkData.events {
		events = append(events, eventImpl{
			event: "put-object",
			data:  obj,
		})
	}

	events = append(events, eventImpl{
		event: "payload-transferred",
		data: framework.PayloadTransferred{
			State:   fdv2SdkData.state,
			Version: 1,
		},
	})

	return events
}

func (s *StreamingService) PushServerIntent(intentCode, intentReason string) {
	s.requireFDv2("server-intent")
	event := eventImpl{
		event: "server-intent",
		data: framework.ServerIntent{
			Payloads: []framework.Payload{
				{
					ID:     "payloadID",
					Target: 1,
					Code:   intentCode,
					Reason: intentReason,
				},
			},
		},
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
		s.debugLogger.Println("Will send server-intent event after connection has started")
	}
}

// PushEvent sends an SSE event to all clients that are currently connected to the stream-- or, if no client
// has connected yet, queues the event so that it will be sent (after the initial data) to the
// first client that connects. (The latter is necessary to avoid race conditions, since even after
// a connection is received on the stream endpoint, it is hard for the test logic to know when the
// HTTP handler has actually created a stream subscription for that connection. If we called
// the eventsource Publish method before a subscription existed, the event would be lost.)
func (s *StreamingService) PushEvent(eventName string, eventData interface{}) {
	event := eventImpl{
		event: eventName,
		data:  eventData,
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

// PushUpdate sends a new version of one flag or segment. The namespace is the FDv2 kind, "flag" or
// "segment". Over FDv2, it sends a put-object event; the SDK applies the change only after a
// payload-transferred event. Over FDv1, it sends a patch event, and the version is the one in the data.
func (s *StreamingService) PushUpdate(namespace, key string, version int, data json.RawMessage) {
	if s.protocol == FDv1 {
		s.PushEvent("patch", s.fdv1PatchData(namespace, key, data))
		return
	}
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
		// Client FDv2 put-object must use the same envelope as initial payload events
		// (see ClientSDKDataToFDv2Events): kind "flag-eval" plus key/version/object.
		eventData = framework.BaseObject{
			Version: version,
			Kind:    "flag-eval",
			Key:     key,
			Object:  data,
		}
	}
	s.PushEvent("put-object", eventData)
}

func (s *StreamingService) PushHeartbeat() {
	s.requireFDv2("heartbeat")
	s.PushEvent("heartbeat", framework.Heartbeat{})
}

func (s *StreamingService) PushError(payloadID, reason string) {
	s.requireFDv2("error")
	s.PushEvent("error", framework.ErrorEvent{
		PayloadID: payloadID,
		Reason:    reason,
	})
}

func (s *StreamingService) PushGoodbye(reason string, silent, catastrophe bool) {
	s.requireFDv2("goodbye")
	s.PushEvent("goodbye", framework.Goodbye{
		Reason:      reason,
		Silent:      silent,
		Catastrophe: catastrophe,
	})
}

func (s *StreamingService) PushPayloadTransferred(state string, version int) {
	s.requireFDv2("payload-transferred")
	eventData := framework.PayloadTransferred{
		State:   state,
		Version: version,
	}
	s.PushEvent("payload-transferred", eventData)
}

// PushDelete sends a deletion of one flag or segment. The namespace is the FDv2 kind, "flag" or
// "segment". Over FDv2, it sends a delete-object event; the SDK applies the change only after a
// payload-transferred event. Over FDv1, it sends a delete event.
func (s *StreamingService) PushDelete(namespace, key string, version int) {
	if s.protocol == FDv1 {
		s.PushEvent("delete", s.fdv1DeleteData(namespace, key, version))
		return
	}
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
		eventData = framework.BaseObject{
			Version: version,
			Kind:    "flag-eval",
			Key:     key,
		}
	}
	s.PushEvent("delete-object", eventData)
}

// PushItemUpdate sends a new version of one flag or segment as a complete change, in the protocol of the
// service. Over FDv2, it sends a put-object event and then a payload-transferred event. Over FDv1, it
// sends a patch event.
func (s *StreamingService) PushItemUpdate(kind, key string, version int, data json.RawMessage) {
	s.PushUpdate(kind, key, version, data)
	s.pushChangeComplete()
}

// PushItemDelete sends a deletion of one flag or segment as a complete change, in the protocol of the
// service. Over FDv2, it sends a delete-object event and then a payload-transferred event. Over FDv1, it
// sends a delete event.
func (s *StreamingService) PushItemDelete(kind, key string, version int) {
	s.PushDelete(kind, key, version)
	s.pushChangeComplete()
}

// PushFullData replaces all of the SDK's data with the specified data, in the protocol of the service.
// Over FDv2, it sends a server-intent event with the xfer-full intent, a put-object event for each item,
// and a payload-transferred event. Over FDv1, it sends a put event.
func (s *StreamingService) PushFullData(data SDKData) {
	if s.protocol == FDv1 {
		s.PushEvent("put", s.fdv1PutData(data))
		return
	}
	fdv2Data := toFDv2SDKData(data, changedPayloadState)
	s.PushServerIntent(fdv2Data.intentCode, fdv2Data.intentReason)
	for _, item := range fdv2Data.events {
		s.PushEvent("put-object", item)
	}
	s.PushPayloadTransferred(fdv2Data.state, s.nextPayloadVersion())
}

// pushChangeComplete ends a change that PushItemUpdate or PushItemDelete started. Over FDv2, it sends a
// payload-transferred event with a new payload version. FDv1 has no such event.
func (s *StreamingService) pushChangeComplete() {
	if s.protocol == FDv1 {
		return
	}
	s.PushPayloadTransferred(changedPayloadState, s.nextPayloadVersion())
}

func (s *StreamingService) nextPayloadVersion() int {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.payloadVersion++
	return s.payloadVersion
}

// fdv1PatchData returns the data of an FDv1 "patch" event. A server-side event has the item in its
// "data" property. A client-side event is the flag evaluation result with its key added.
func (s *StreamingService) fdv1PatchData(kind, key string, data json.RawMessage) interface{} {
	if s.sdkKind.IsServerSide() {
		return map[string]interface{}{
			"path": "/" + fdv1Namespace(kind) + "/" + key,
			"data": data,
		}
	}
	if kind != "flag" {
		panic(errClientSideStreamCanOnlyUseFlags)
	}
	var props map[string]json.RawMessage
	if err := json.Unmarshal(data, &props); err != nil {
		panic(fmt.Sprintf("test logic error: an FDv1 client-side patch needs a JSON object: %s", err))
	}
	props["key"] = jsonhelpers.ToJSON(key)
	return props
}

// fdv1DeleteData returns the data of an FDv1 "delete" event.
func (s *StreamingService) fdv1DeleteData(kind, key string, version int) interface{} {
	if s.sdkKind.IsServerSide() {
		return map[string]interface{}{
			"path":    "/" + fdv1Namespace(kind) + "/" + key,
			"version": version,
		}
	}
	if kind != "flag" {
		panic(errClientSideStreamCanOnlyUseFlags)
	}
	return map[string]interface{}{
		"key":     key,
		"version": version,
	}
}

func (s *StreamingService) Replay(channel, id string) chan eventsource.Event {
	events := s.initialEvents()

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

func (e eventImpl) Event() string { return e.event }
func (e eventImpl) Id() string    { return "" } //nolint:revive // required by interface
func (e eventImpl) Data() string {
	if raw, ok := e.data.(json.RawMessage); ok {
		return string(raw) // this allows us to pass malformed data that json.Marshal wouldn't allow
	}
	bytes, _ := json.Marshal(e.data)
	return string(bytes)
}
