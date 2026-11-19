package mockld

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v3/framework/helpers"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startEventsService starts an EventsService for the SDK kind on a test server.
func startEventsService(t *testing.T, sdkKind SDKKind) (*EventsService, *httptest.Server) {
	testLog := ldlogtest.NewMockLog()
	t.Cleanup(func() { testLog.DumpIfTestFailed(t) })
	service := NewEventsService(sdkKind, testLog.Loggers.ForLevel(ldlog.Debug), false)
	server := httptest.NewServer(service)
	t.Cleanup(server.Close)
	return service, server
}

// postToEventsService posts the body to a path of the server, with the given headers, and returns
// the HTTP status.
func postToEventsService(t *testing.T, server *httptest.Server, path, body string, headers ...string) int {
	req, err := http.NewRequest(http.MethodPost, server.URL+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestEventsServiceRecordsDiagnosticEventForEachSDKKind(t *testing.T) {
	for _, p := range []struct {
		name    string
		sdkKind SDKKind
		path    string
	}{
		{"server-side", ServerSideSDK, "/diagnostic"},
		{"php", PHPSDK, "/diagnostic"},
		{"mobile", MobileSDK, "/mobile/events/diagnostic"},
		{"roku", RokuSDK, "/mobile/events/diagnostic"},
		{"js client", JSClientSDK, "/events/diagnostic/my-env-id"},
	} {
		t.Run(p.name, func(t *testing.T) {
			service, server := startEventsService(t, p.sdkKind)

			assert.Equal(t, http.StatusAccepted, postToEventsService(t, server, p.path, `{"kind":"diagnostic-init","creationDate":1}`))

			event := helpers.TryReceive(service.Diagnostics.Payloads, time.Second)
			require.True(t, event.IsDefined(), "diagnostic event from the diagnostic route")
			assert.JSONEq(t, `{"kind":"diagnostic-init","creationDate":1}`, event.Value().JSONString())

			_, analytics := service.AwaitAnalyticsEventPayload(time.Millisecond * 50)
			assert.False(t, analytics, "analytics payload from a post to the diagnostic route")
		})
	}
}

func TestEventsServiceDoesNotRecordDiagnosticPostThatIsNotAnObject(t *testing.T) {
	for _, body := range []string{`[{"kind":"diagnostic-init"}]`, `"diagnostic-init"`, `not json`, ``} {
		t.Run(body, func(t *testing.T) {
			service, server := startEventsService(t, ServerSideSDK)

			assert.Equal(t, http.StatusAccepted, postToEventsService(t, server, "/diagnostic", body))

			event := helpers.TryReceive(service.Diagnostics.Payloads, time.Millisecond*50)
			assert.False(t, event.IsDefined(), "diagnostic event from a body that is not a JSON object")
		})
	}
}

func TestEventsServiceAnswersDiagnosticPostThatExpectsContinue(t *testing.T) {
	service, server := startEventsService(t, ServerSideSDK)

	req, err := http.NewRequest(http.MethodPost, server.URL+"/diagnostic",
		bytes.NewReader([]byte(`{"kind":"diagnostic-init"}`)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Expect", "100-continue")
	// The client waits this long for "100 Continue" before it sends the body anyway.
	client := &http.Client{Transport: &http.Transport{ExpectContinueTimeout: time.Second * 5}}

	start := time.Now()
	resp, err := client.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
	assert.Less(t, time.Since(start), time.Second, "the client waited for its expect-continue timeout")
	assert.True(t, helpers.TryReceive(service.Diagnostics.Payloads, time.Second).IsDefined())
}

func TestEventsServiceDoesNotBlockWhenDiagnosticChannelIsFull(t *testing.T) {
	for _, p := range []struct {
		name   string
		path   string
		body   string
		status int
		ch     func(*EventsService) chan Event
	}{
		{"diagnostic route", "/diagnostic", `{"kind":"diagnostic"}`, http.StatusAccepted,
			func(s *EventsService) chan Event { return s.Diagnostics.Payloads }},
		{"analytics route", "/bulk", `{"kind":"diagnostic"}`, http.StatusBadRequest,
			func(s *EventsService) chan Event { return s.Diagnostics.Misrouted }},
	} {
		t.Run(p.name, func(t *testing.T) {
			service, server := startEventsService(t, ServerSideSDK)

			// Nothing reads the channel, so the posts after the channel is full have no free slot.
			for i := 0; i < eventsChannelBufferSize+5; i++ {
				assert.Equal(t, p.status, postToEventsService(t, server, p.path, p.body))
			}
			assert.Len(t, p.ch(service), eventsChannelBufferSize)
		})
	}
}

func TestEventsServiceRecordsDiagnosticObjectPostedToAnalyticsRoute(t *testing.T) {
	for _, p := range []struct {
		name    string
		sdkKind SDKKind
		path    string
	}{
		{"server-side", ServerSideSDK, "/bulk"},
		{"mobile", MobileSDK, "/mobile"},
		{"mobile events", MobileSDK, "/mobile/events"},
		{"mobile events bulk", MobileSDK, "/mobile/events/bulk"},
		{"js client", JSClientSDK, "/events/bulk/my-env-id"},
	} {
		t.Run(p.name, func(t *testing.T) {
			service, server := startEventsService(t, p.sdkKind)

			// The body is one JSON object, not an array of events.
			assert.Equal(t, http.StatusBadRequest, postToEventsService(t, server, p.path, `{"kind":"diagnostic-init","creationDate":1}`))

			event, misrouted := service.Diagnostics.AwaitMisrouted(time.Second)
			require.True(t, misrouted, "misrouted event from a diagnostic object on the analytics route")
			assert.Equal(t, "diagnostic-init", event.Kind())

			received := helpers.TryReceive(service.Diagnostics.Payloads, time.Millisecond*50)
			assert.False(t, received.IsDefined(), "diagnostic event from a post to the analytics route")
			_, analytics := service.AwaitAnalyticsEventPayload(time.Millisecond * 50)
			assert.False(t, analytics, "analytics payload from a body that is not an array")
		})
	}
}

func TestEventsServiceRecordsDiagnosticEventInsideAnalyticsPayload(t *testing.T) {
	service, server := startEventsService(t, ServerSideSDK)

	assert.Equal(t, http.StatusAccepted, postToEventsService(t, server, "/bulk", `[{"kind":"custom"},{"kind":"diagnostic"}]`))

	// The analytics payload is delivered unchanged.
	events, received := service.AwaitAnalyticsEventPayload(time.Second)
	require.True(t, received)
	assert.Len(t, events, 2)

	// The misrouted event is available as soon as the payload is available.
	select {
	case event := <-service.Diagnostics.Misrouted:
		assert.Equal(t, "diagnostic", event.Kind())
	default:
		assert.Fail(t, "misrouted event was not recorded before the analytics payload was delivered")
	}

	_, extra := service.Diagnostics.AwaitMisrouted(time.Millisecond * 50)
	assert.False(t, extra, "misrouted event from an analytics event")
}

func TestEventsServiceRecordsMisroutedDiagnosticEventWithSeenPayloadID(t *testing.T) {
	service, server := startEventsService(t, ServerSideSDK)

	postToEventsService(t, server, "/bulk", `[{"kind":"identify"}]`, "X-LaunchDarkly-Payload-ID", "same-id")
	postToEventsService(t, server, "/bulk", `{"kind":"diagnostic"}`, "X-LaunchDarkly-Payload-ID", "same-id")

	event, misrouted := service.Diagnostics.AwaitMisrouted(time.Second)
	require.True(t, misrouted, "misrouted event from a post with a payload ID that was already seen")
	assert.Equal(t, "diagnostic", event.Kind())
}

func TestEventsServiceDoesNotRecordMisroutedEventForCorrectPosts(t *testing.T) {
	for _, p := range []struct {
		name           string
		path           string
		body           string
		expectedStatus int
	}{
		{"analytics events", "/bulk", `[{"kind":"identify"},{"kind":"custom"}]`, http.StatusAccepted},
		{"empty array", "/bulk", `[]`, http.StatusAccepted},
		{"object that is not a diagnostic event", "/bulk", `{"kind":"custom"}`, http.StatusBadRequest},
		{"malformed JSON", "/bulk", `{"kind":"diagnostic-init"`, http.StatusBadRequest},
		{"diagnostic event on the diagnostic route", "/diagnostic", `{"kind":"diagnostic-init"}`, http.StatusAccepted},
	} {
		t.Run(p.name, func(t *testing.T) {
			service, server := startEventsService(t, ServerSideSDK)

			assert.Equal(t, p.expectedStatus, postToEventsService(t, server, p.path, p.body))

			_, misrouted := service.Diagnostics.AwaitMisrouted(time.Millisecond * 50)
			assert.False(t, misrouted, "misrouted event")
		})
	}
}
