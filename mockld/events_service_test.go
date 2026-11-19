package mockld

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func postToEventsService(t *testing.T, server *httptest.Server, path string, body string) int {
	resp, err := http.Post(server.URL+path, "application/json", strings.NewReader(body))
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
			testLog := ldlogtest.NewMockLog()
			defer testLog.DumpIfTestFailed(t)
			service := NewEventsService(p.sdkKind, testLog.Loggers.ForLevel(ldlog.Debug), false)

			httphelpers.WithServer(service, func(server *httptest.Server) {
				status := postToEventsService(t, server, p.path, `{"kind":"diagnostic-init","creationDate":1}`)
				assert.Equal(t, http.StatusAccepted, status)

				event, received := service.AwaitDiagnosticEventPayload(time.Second)
				require.True(t, received, "diagnostic event from the diagnostic route")
				assert.Equal(t, "diagnostic-init", event.Kind())
				assert.JSONEq(t, `{"kind":"diagnostic-init","creationDate":1}`, event.JSONString())

				_, analytics := service.AwaitAnalyticsEventPayload(time.Millisecond * 50)
				assert.False(t, analytics, "analytics payload from a post to the diagnostic route")
			})
		})
	}
}

func TestEventsServiceDoesNotRecordDiagnosticPostThatIsNotAnObject(t *testing.T) {
	for _, body := range []string{`[{"kind":"diagnostic-init"}]`, `"diagnostic-init"`, `not json`, ``} {
		t.Run(body, func(t *testing.T) {
			testLog := ldlogtest.NewMockLog()
			defer testLog.DumpIfTestFailed(t)
			service := NewEventsService(ServerSideSDK, testLog.Loggers.ForLevel(ldlog.Debug), false)

			httphelpers.WithServer(service, func(server *httptest.Server) {
				status := postToEventsService(t, server, "/diagnostic", body)
				assert.Equal(t, http.StatusAccepted, status)

				_, received := service.AwaitDiagnosticEventPayload(time.Millisecond * 50)
				assert.False(t, received, "diagnostic event from a body that is not a JSON object")
			})
		})
	}
}

func TestEventsServiceDoesNotBlockWhenDiagnosticChannelIsFull(t *testing.T) {
	testLog := ldlogtest.NewMockLog()
	defer testLog.DumpIfTestFailed(t)
	service := NewEventsService(ServerSideSDK, testLog.Loggers.ForLevel(ldlog.Debug), false)

	httphelpers.WithServer(service, func(server *httptest.Server) {
		// Nothing reads the channel, so the posts after the channel is full have no free slot.
		for i := 0; i < eventsChannelBufferSize+5; i++ {
			assert.Equal(t, http.StatusAccepted,
				postToEventsService(t, server, "/diagnostic", `{"kind":"diagnostic"}`))
		}
		assert.Len(t, service.DiagnosticEventPayloads, eventsChannelBufferSize)
	})
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
			testLog := ldlogtest.NewMockLog()
			defer testLog.DumpIfTestFailed(t)
			service := NewEventsService(p.sdkKind, testLog.Loggers.ForLevel(ldlog.Debug), false)

			httphelpers.WithServer(service, func(server *httptest.Server) {
				// The body is one JSON object, not an array of events.
				status := postToEventsService(t, server, p.path, `{"kind":"diagnostic-init","creationDate":1}`)
				assert.Equal(t, http.StatusBadRequest, status)

				event, misrouted := service.AwaitMisroutedDiagnosticPayload(time.Second)
				require.True(t, misrouted, "misrouted event from a diagnostic object on the analytics route")
				assert.Equal(t, "diagnostic-init", event.Kind())

				_, received := service.AwaitDiagnosticEventPayload(time.Millisecond * 50)
				assert.False(t, received, "diagnostic event from a post to the analytics route")
				_, analytics := service.AwaitAnalyticsEventPayload(time.Millisecond * 50)
				assert.False(t, analytics, "analytics payload from a body that is not an array")
			})
		})
	}
}

func TestEventsServiceRecordsDiagnosticEventInsideAnalyticsPayload(t *testing.T) {
	testLog := ldlogtest.NewMockLog()
	defer testLog.DumpIfTestFailed(t)
	service := NewEventsService(ServerSideSDK, testLog.Loggers.ForLevel(ldlog.Debug), false)

	httphelpers.WithServer(service, func(server *httptest.Server) {
		status := postToEventsService(t, server, "/bulk", `[{"kind":"custom"},{"kind":"diagnostic"}]`)
		assert.Equal(t, http.StatusAccepted, status)

		// The analytics payload is delivered unchanged.
		events, received := service.AwaitAnalyticsEventPayload(time.Second)
		require.True(t, received)
		assert.Len(t, events, 2)

		// The misrouted event is available as soon as the payload is available.
		select {
		case event := <-service.MisroutedDiagnosticPayloads:
			assert.Equal(t, "diagnostic", event.Kind())
		default:
			assert.Fail(t, "misrouted event was not recorded before the analytics payload was delivered")
		}

		_, extra := service.AwaitMisroutedDiagnosticPayload(time.Millisecond * 50)
		assert.False(t, extra, "misrouted event from an analytics event")
	})
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
			testLog := ldlogtest.NewMockLog()
			defer testLog.DumpIfTestFailed(t)
			service := NewEventsService(ServerSideSDK, testLog.Loggers.ForLevel(ldlog.Debug), false)

			httphelpers.WithServer(service, func(server *httptest.Server) {
				status := postToEventsService(t, server, p.path, p.body)
				assert.Equal(t, p.expectedStatus, status)

				_, misrouted := service.AwaitMisroutedDiagnosticPayload(time.Millisecond * 50)
				assert.False(t, misrouted, "misrouted event")
			})
		})
	}
}

func TestEventsServiceDoesNotBlockWhenMisroutedDiagnosticChannelIsFull(t *testing.T) {
	testLog := ldlogtest.NewMockLog()
	defer testLog.DumpIfTestFailed(t)
	service := NewEventsService(ServerSideSDK, testLog.Loggers.ForLevel(ldlog.Debug), false)

	httphelpers.WithServer(service, func(server *httptest.Server) {
		// Nothing reads the channel, so the posts after the channel is full have no free slot.
		for i := 0; i < eventsChannelBufferSize+5; i++ {
			assert.Equal(t, http.StatusBadRequest,
				postToEventsService(t, server, "/bulk", `{"kind":"diagnostic"}`))
		}
		assert.Len(t, service.MisroutedDiagnosticPayloads, eventsChannelBufferSize)
	})
}
