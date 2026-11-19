package sdktests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v3/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withMobileEventSink runs the action with an SDKEventSink for a mobile SDK, and with a function
// that posts a body to a path of the events service of that sink. The sink has no mock endpoint,
// so the action must not call Endpoint or Configure.
func withMobileEventSink(t *testing.T, action func(sink *SDKEventSink, post func(path, body string))) {
	testLog := ldlogtest.NewMockLog()
	defer testLog.DumpIfTestFailed(t)
	service := mockld.NewEventsService(mockld.MobileSDK, testLog.Loggers.ForLevel(ldlog.Debug), false)
	sink := &SDKEventSink{eventsService: service}

	httphelpers.WithServer(service, func(server *httptest.Server) {
		action(sink, func(path, body string) {
			resp, err := http.Post(server.URL+path, "application/json", strings.NewReader(body))
			require.NoError(t, err)
			_ = resp.Body.Close()
		})
	})
}

func TestSDKEventSinkExpectDiagnosticEvent(t *testing.T) {
	t.Run("returns the event from the diagnostic route", func(t *testing.T) {
		withMobileEventSink(t, func(sink *SDKEventSink, post func(path, body string)) {
			post("/mobile/events/diagnostic", `{"kind":"diagnostic-init"}`)

			var recorder helpers.TestRecorder
			event := sink.ExpectDiagnosticEvent(&recorder, time.Second)

			assert.NoError(t, recorder.Err())
			assert.False(t, recorder.Terminated)
			assert.Equal(t, "diagnostic-init", event.Kind())
		})
	})

	t.Run("fails if the diagnostic object goes to the analytics route", func(t *testing.T) {
		withMobileEventSink(t, func(sink *SDKEventSink, post func(path, body string)) {
			post("/mobile/events/bulk", `{"kind":"diagnostic-init"}`)

			var recorder helpers.TestRecorder
			start := time.Now()
			_ = sink.ExpectDiagnosticEvent(&recorder, time.Second*10)

			assert.True(t, recorder.Terminated)
			require.Error(t, recorder.Err())
			assert.Contains(t, recorder.Err().Error(), "received a diagnostic event on the analytics events route")
			assert.Contains(t, recorder.Err().Error(), "diagnostic-init")
			// The failure does not wait for the timeout.
			assert.Less(t, time.Since(start), time.Second*5)
		})
	})

	t.Run("fails if no diagnostic event arrives", func(t *testing.T) {
		withMobileEventSink(t, func(sink *SDKEventSink, post func(path, body string)) {
			post("/mobile/events/bulk", `[{"kind":"identify"}]`)

			var recorder helpers.TestRecorder
			_ = sink.ExpectDiagnosticEvent(&recorder, time.Millisecond*50)

			assert.True(t, recorder.Terminated)
			require.Error(t, recorder.Err())
			assert.Contains(t, recorder.Err().Error(), "timed out waiting for a diagnostic event")
		})
	})
}

func TestSDKEventSinkExpectNoMisroutedDiagnosticEvents(t *testing.T) {
	t.Run("passes if diagnostic events go to the diagnostic route only", func(t *testing.T) {
		withMobileEventSink(t, func(sink *SDKEventSink, post func(path, body string)) {
			post("/mobile/events/diagnostic", `{"kind":"diagnostic-init"}`)
			post("/mobile/events/bulk", `[{"kind":"identify"}]`)

			var recorder helpers.TestRecorder
			sink.ExpectNoMisroutedDiagnosticEvents(&recorder, time.Millisecond*50)

			assert.NoError(t, recorder.Err())
			assert.False(t, recorder.Terminated)
		})
	})

	t.Run("fails if the diagnostic object goes to the analytics route", func(t *testing.T) {
		withMobileEventSink(t, func(sink *SDKEventSink, post func(path, body string)) {
			post("/mobile/events/bulk", `{"kind":"diagnostic-init"}`)

			var recorder helpers.TestRecorder
			sink.ExpectNoMisroutedDiagnosticEvents(&recorder, time.Second)

			assert.True(t, recorder.Terminated)
			require.Error(t, recorder.Err())
			assert.Contains(t, recorder.Err().Error(), "received a diagnostic event on the analytics events route")
		})
	})

	t.Run("fails if a diagnostic event is inside an analytics payload", func(t *testing.T) {
		withMobileEventSink(t, func(sink *SDKEventSink, post func(path, body string)) {
			post("/mobile/events/bulk", `[{"kind":"identify"},{"kind":"diagnostic"}]`)

			var recorder helpers.TestRecorder
			sink.ExpectNoMisroutedDiagnosticEvents(&recorder, time.Second)

			assert.True(t, recorder.Terminated)
			require.Error(t, recorder.Err())
			assert.Contains(t, recorder.Err().Error(), "received a diagnostic event on the analytics events route")
		})
	})
}
