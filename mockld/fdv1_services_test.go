package mockld

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	h "github.com/launchdarkly/sdk-test-harness/v3/framework/helpers"

	"github.com/launchdarkly/eventsource"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	serverFlag1    = json.RawMessage(`{"key":"flag1","version":1}`)
	serverSegment1 = json.RawMessage(`{"key":"segment1","version":1}`)
)

func serverDataWithFlagAndSegment() ServerSDKData {
	return NewServerSDKDataBuilder().RawFlag("flag1", serverFlag1).RawSegment("segment1", serverSegment1).Build()
}

func clientDataWithFlag() ClientSDKData {
	return NewClientSDKDataBuilder().FlagWithValue("flag1", 1, ldvalue.String("yes"), 0).Build()
}

const (
	expectedFDv1ServerPayload = `{"flags":{"flag1":{"key":"flag1","version":1}},` +
		`"segments":{"segment1":{"key":"segment1","version":1}}}`
	expectedFDv1ClientPayload = `{"flag1":{"value":"yes","variation":0,"reason":null,"version":1,` +
		`"flagVersion":null,"trackEvents":false,"trackReason":false,"debugEventsUntilDate":null}}`
)

func newTestLog(t *testing.T) *ldlogtest.MockLog {
	testLog := ldlogtest.NewMockLog()
	testLog.Loggers.SetMinLevel(ldlog.Debug)
	t.Cleanup(func() { testLog.DumpIfTestFailed(t) })
	return testLog
}

func subscribe(t *testing.T, server *httptest.Server, method, path string) *eventsource.Stream {
	req, _ := http.NewRequest(method, server.URL+path, nil)
	stream, err := eventsource.SubscribeWithRequest("", req)
	require.NoError(t, err)
	t.Cleanup(stream.Close)
	return stream
}

func requireStreamEvent(t *testing.T, stream *eventsource.Stream, name, expectedJSON string) {
	t.Helper()
	event := h.RequireValueWithMessage(t, stream.Events, time.Second*5, "timed out waiting for event")
	assert.Equal(t, name, event.Event())
	m.In(t).Assert(event.Data(), m.JSONStrEqual(expectedJSON))
}

func TestFDv1StreamingServiceSendsPutEvent(t *testing.T) {
	serverPut := `{"data":` + expectedFDv1ServerPayload + `}`
	emptyServerPut := `{"data":{"flags":{},"segments":{}}}`

	for _, p := range []struct {
		name                  string
		sdkKind               SDKKind
		emptyData, data       SDKData
		method, path          string
		expectedEmpty, expect string
	}{
		{"server-side, server data", ServerSideSDK, NewServerSDKDataBuilder().Build(),
			serverDataWithFlagAndSegment(), "GET", "/all", emptyServerPut, serverPut},
		{"server-side, FDv2 data", ServerSideSDK, EmptyServerSDKData(),
			FDv2SDKDataFromServerSDKData(serverDataWithFlagAndSegment(), "xfer-full", "initial", "initial"),
			"GET", "/all", emptyServerPut, serverPut},
		{"mobile GET", MobileSDK, EmptyClientSDKData(), clientDataWithFlag(),
			"GET", "/meval/fakeuserdata", `{}`, expectedFDv1ClientPayload},
		{"mobile REPORT", MobileSDK, EmptyClientSDKData(), clientDataWithFlag(),
			"REPORT", "/meval", `{}`, expectedFDv1ClientPayload},
		{"JS client GET", JSClientSDK, EmptyClientSDKData(),
			FDv2SDKDataFromClientSDKData(clientDataWithFlag(), "xfer-full", "initial", "initial"),
			"GET", "/eval/fakeid/fakeuserdata", `{}`, expectedFDv1ClientPayload},
		{"JS client REPORT", JSClientSDK, EmptyClientSDKData(), clientDataWithFlag(),
			"REPORT", "/eval/fakeid", `{}`, expectedFDv1ClientPayload},
	} {
		t.Run(p.name, func(t *testing.T) {
			service := NewStreamingServiceForProtocol(FDv1, p.emptyData, p.sdkKind,
				newTestLog(t).Loggers.ForLevel(ldlog.Debug))
			httphelpers.WithServer(service, func(server *httptest.Server) {
				stream := subscribe(t, server, p.method, p.path)
				requireStreamEvent(t, stream, "put", p.expectedEmpty)

				go func() {
					service.SetInitialData(p.data)
					service.RefreshAll()
				}()
				requireStreamEvent(t, stream, "put", p.expect)
			})
		})
	}
}

func TestFDv1StreamingServiceDoesNotServeFDv2Paths(t *testing.T) {
	service := NewStreamingServiceForProtocol(FDv1, EmptyServerSDKData(), ServerSideSDK,
		newTestLog(t).Loggers.ForLevel(ldlog.Debug))
	httphelpers.WithServer(service, func(server *httptest.Server) {
		resp, err := http.Get(server.URL + StreamingPathServerSide)
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func TestStreamingServiceProtocolNeutralPushes(t *testing.T) {
	flag2 := json.RawMessage(`{"key":"flag2","version":2}`)
	clientFlag2 := json.RawMessage(`{"value":"no","variation":1,"version":2}`)
	newServerData := NewServerSDKDataBuilder().RawFlag("flag2", flag2).Build()
	newClientData := NewClientSDKDataBuilder().FlagWithValue("flag2", 2, ldvalue.String("no"), 1).Build()

	type expectedEvent struct{ name, data string }
	for _, p := range []struct {
		name          string
		protocol      DataProtocol
		sdkKind       SDKKind
		path          string
		initialEvents int // the number of events that deliver the initial data
		push          func(*StreamingService)
		expectEvents  []expectedEvent
	}{
		{"FDv1 server-side", FDv1, ServerSideSDK, "/all", 1,
			func(s *StreamingService) {
				s.PushItemUpdate("flag", "flag2", 2, flag2)
				s.PushItemDelete("segment", "segment1", 3)
				s.PushFullData(newServerData)
			},
			[]expectedEvent{
				{"patch", `{"path":"/flags/flag2","data":{"key":"flag2","version":2}}`},
				{"delete", `{"path":"/segments/segment1","version":3}`},
				{"put", `{"data":{"flags":{"flag2":{"key":"flag2","version":2}},"segments":{}}}`},
			}},
		{"FDv1 client-side", FDv1, MobileSDK, "/meval/fakeuserdata", 1,
			func(s *StreamingService) {
				s.PushItemUpdate("flag", "flag2", 2, clientFlag2)
				s.PushItemDelete("flag", "flag1", 3)
				s.PushFullData(newClientData)
			},
			[]expectedEvent{
				{"patch", `{"key":"flag2","value":"no","variation":1,"version":2}`},
				{"delete", `{"key":"flag1","version":3}`},
				{"put", `{"flag2":{"value":"no","variation":1,"reason":null,"version":2,"flagVersion":null,` +
					`"trackEvents":false,"trackReason":false,"debugEventsUntilDate":null}}`},
			}},
		// server-intent, a put-object for the flag and for the segment, and payload-transferred
		{"FDv2 server-side", FDv2, ServerSideSDK, "/sdk/stream", 4,
			func(s *StreamingService) {
				s.PushItemUpdate("flag", "flag2", 2, flag2)
				s.PushItemDelete("segment", "segment1", 3)
				s.PushFullData(newServerData)
			},
			[]expectedEvent{
				{"put-object", `{"version":2,"kind":"flag","key":"flag2","object":{"key":"flag2","version":2}}`},
				{"payload-transferred", `{"state":"updated","version":2}`},
				{"delete-object", `{"version":3,"kind":"segment","key":"segment1"}`},
				{"payload-transferred", `{"state":"updated","version":3}`},
				{"server-intent", `{"payloads":[{"id":"payloadID","target":1,"intentCode":"xfer-full",` +
					`"reason":"stale"}]}`},
				{"put-object", `{"version":2,"kind":"flag","key":"flag2","object":{"key":"flag2","version":2}}`},
				{"payload-transferred", `{"state":"updated","version":4}`},
			}},
	} {
		t.Run(p.name, func(t *testing.T) {
			var initialData SDKData = serverDataWithFlagAndSegment()
			if p.sdkKind.IsClientSide() {
				initialData = clientDataWithFlag()
			}
			service := NewStreamingServiceForProtocol(p.protocol, toFDv2SDKData(initialData, "initial"), p.sdkKind,
				newTestLog(t).Loggers.ForLevel(ldlog.Debug))
			httphelpers.WithServer(service, func(server *httptest.Server) {
				stream := subscribe(t, server, "GET", p.path)
				p.push(service)

				var events []expectedEvent
				deadline := time.After(time.Second * 5)
				for len(events) < p.initialEvents+len(p.expectEvents) {
					select {
					case e := <-stream.Events:
						events = append(events, expectedEvent{e.Event(), e.Data()})
					case <-deadline:
						require.Fail(t, "timed out waiting for events", "got %v", events)
					}
				}
				events = events[p.initialEvents:]
				for i, expected := range p.expectEvents {
					assert.Equal(t, expected.name, events[i].name, "event %d", i)
					m.In(t).Assert(events[i].data, m.JSONStrEqual(expected.data))
				}
			})
		})
	}
}

func TestFDv1StreamingServiceRejectsFDv2Events(t *testing.T) {
	service := NewStreamingServiceForProtocol(FDv1, EmptyServerSDKData(), ServerSideSDK,
		newTestLog(t).Loggers.ForLevel(ldlog.Debug))
	for name, push := range map[string]func(){
		"server-intent":       func() { service.PushServerIntent("xfer-full", "stale") },
		"payload-transferred": func() { service.PushPayloadTransferred("updated", 2) },
		"goodbye":             func() { service.PushGoodbye("reason", false, false) },
		"error":               func() { service.PushError("payloadID", "reason") },
		"heartbeat":           service.PushHeartbeat,
	} {
		t.Run(name, func(t *testing.T) {
			assert.Panics(t, push)
		})
	}
}

func TestFDv1DataCannotRepresentPartialFDv2Payload(t *testing.T) {
	data := NewFDv2SDKData("xfer-changes", "", "state", nil)
	assert.Panics(t, func() { fdv1Payload(data, ServerSideSDK) })
}

func TestFDv1PollingService(t *testing.T) {
	for _, p := range []struct {
		name          string
		sdkKind       SDKKind
		data          SDKData
		method, path  string
		expectedJSON  string
		expectedEmpty bool
	}{
		{"server-side", ServerSideSDK, serverDataWithFlagAndSegment(), "GET", "/sdk/latest-all",
			expectedFDv1ServerPayload, false},
		{"mobile GET context", MobileSDK, clientDataWithFlag(), "GET", "/msdk/evalx/contexts/fakeuserdata",
			expectedFDv1ClientPayload, false},
		{"mobile REPORT context", MobileSDK, clientDataWithFlag(), "REPORT", "/msdk/evalx/context",
			expectedFDv1ClientPayload, false},
		{"mobile GET user", MobileSDK, clientDataWithFlag(), "GET", "/msdk/evalx/users/fakeuserdata",
			expectedFDv1ClientPayload, false},
		{"mobile REPORT user", MobileSDK, clientDataWithFlag(), "REPORT", "/msdk/evalx/user",
			expectedFDv1ClientPayload, false},
		{"JS client GET context", JSClientSDK, clientDataWithFlag(), "GET", "/sdk/evalx/fakeid/contexts/fakeuserdata",
			expectedFDv1ClientPayload, false},
		{"JS client REPORT context", JSClientSDK, clientDataWithFlag(), "REPORT", "/sdk/evalx/fakeid/context",
			expectedFDv1ClientPayload, false},
		{"JS client GET user", JSClientSDK, clientDataWithFlag(), "GET", "/sdk/evalx/fakeid/users/fakeuserdata",
			expectedFDv1ClientPayload, false},
		{"JS client REPORT user", JSClientSDK, clientDataWithFlag(), "REPORT", "/sdk/evalx/fakeid/user",
			expectedFDv1ClientPayload, false},
		{"PHP all flags", PHPSDK, serverDataWithFlagAndSegment(), "GET", "/sdk/flags",
			`{"flag1":{"key":"flag1","version":1}}`, false},
		{"PHP flag", PHPSDK, serverDataWithFlagAndSegment(), "GET", "/sdk/flags/flag1",
			`{"key":"flag1","version":1}`, false},
		{"PHP segment", PHPSDK, serverDataWithFlagAndSegment(), "GET", "/sdk/segments/segment1",
			`{"key":"segment1","version":1}`, false},
		{"PHP unknown flag", PHPSDK, serverDataWithFlagAndSegment(), "GET", "/sdk/flags/unknown", "", true},
		{"server-side FDv2 path", ServerSideSDK, serverDataWithFlagAndSegment(), "GET", "/sdk/poll", "", true},
	} {
		t.Run(p.name, func(t *testing.T) {
			service := NewPollingServiceForProtocol(FDv1, toFDv2SDKData(p.data, "initial"), p.sdkKind,
				newTestLog(t).Loggers.ForLevel(ldlog.Debug))
			httphelpers.WithServer(service, func(server *httptest.Server) {
				req, _ := http.NewRequest(p.method, server.URL+p.path, strings.NewReader("{}"))
				resp, err := http.DefaultClient.Do(req)
				require.NoError(t, err)
				defer resp.Body.Close()
				if p.expectedEmpty {
					assert.Equal(t, http.StatusNotFound, resp.StatusCode)
					return
				}
				require.Equal(t, http.StatusOK, resp.StatusCode)
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				m.In(t).Assert(string(body), m.JSONStrEqual(p.expectedJSON))
			})
		})
	}
}
