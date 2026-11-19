package sdktests

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"

	"github.com/stretchr/testify/assert"
)

// fakeMobileSDKService is a minimal SDK test service for a mobile SDK. It does not run an SDK. When
// diagnostics are enabled, it posts a diagnostic-init event to diagnosticPath. On a flush, it posts
// flushBody to the analytics events route.
type fakeMobileSDKService struct {
	capabilities   []string
	diagnosticPath string
	flushBody      string
	eventsBaseURI  string
	lock           sync.Mutex
}

func (f *fakeMobileSDKService) post(path string, body string) {
	f.lock.Lock()
	url := f.eventsBaseURI + path
	f.lock.Unlock()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err == nil {
		_ = resp.Body.Close()
	}
}

func (f *fakeMobileSDKService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	switch {
	case r.Method == http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "fake-mobile-sdk", "capabilities": f.capabilities})
	case r.Method == http.MethodPost && r.URL.Path == "/":
		var params servicedef.CreateInstanceParams
		if err := json.Unmarshal(body, &params); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		eventsConfig := params.Configuration.Events.Value()
		f.lock.Lock()
		f.eventsBaseURI = strings.TrimSuffix(eventsConfig.BaseURI, "/")
		f.lock.Unlock()
		if eventsConfig.EnableDiagnostics {
			f.post(f.diagnosticPath, `{"kind":"diagnostic-init"}`)
		}
		w.Header().Set("Location", "/clients/1")
		w.WriteHeader(http.StatusCreated)
	case r.Method == http.MethodPost:
		var command servicedef.CommandParams
		if err := json.Unmarshal(body, &command); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if command.Command == servicedef.CommandFlushEvents {
			f.post("/mobile/events/bulk", f.flushBody)
		}
		w.WriteHeader(http.StatusAccepted)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// ranTest returns true if the results contain a test with the given ID. The results do not contain
// a test that was skipped.
func ranTest(results ldtest.Results, id string) bool {
	for _, result := range results.Tests {
		if result.TestID.String() == id {
			return true
		}
	}
	return false
}

func TestDiagnosticEventsContractTest(t *testing.T) {
	const subtestID = "diagnostic events/init event is sent to the diagnostic route only"
	const analyticsEvents = `[{"kind":"identify"},{"kind":"custom"}]`
	withoutCapability := []string{servicedef.CapabilityClientSide, servicedef.CapabilityMobile}
	allCapabilities := []string{servicedef.CapabilityClientSide, servicedef.CapabilityMobile,
		servicedef.CapabilityDiagnosticEvents}

	for _, p := range []struct {
		name           string
		capabilities   []string
		diagnosticPath string
		flushBody      string
		wantOK         bool
		wantRan        bool
	}{
		{"passes for an SDK that uses the diagnostic route",
			allCapabilities, "/mobile/events/diagnostic", analyticsEvents, true, true},
		{"fails for an SDK that posts the diagnostic object to the analytics route",
			allCapabilities, "/mobile", analyticsEvents, false, true},
		{"fails for an SDK that puts a diagnostic event in the analytics payload",
			allCapabilities, "/mobile/events/diagnostic", `[{"kind":"identify"},{"kind":"diagnostic"}]`, false, true},
		{"does not run without the capability",
			withoutCapability, "/mobile", analyticsEvents, true, false},
	} {
		t.Run(p.name, func(t *testing.T) {
			testHarness := newTestHarnessWithFakeService(t, &fakeMobileSDKService{
				capabilities:   p.capabilities,
				diagnosticPath: p.diagnosticPath,
				flushBody:      p.flushBody,
			})
			logger := &errorRecordingTestLogger{}

			results := ldtest.Run(
				ldtest.TestConfiguration{
					Capabilities: testHarness.TestServiceInfo().Capabilities,
					Context:      SDKTestContext{harness: testHarness, sdkKind: mockld.MobileSDK},
					TestLogger:   logger,
				},
				func(t *ldtest.T) {
					t.Run("diagnostic events", func(t *ldtest.T) {
						NewCommonEventTests(t, "TestDiagnosticEventsContractTest").DiagnosticEvents(t)
					})
				})

			var messages []string
			for _, err := range logger.errors {
				messages = append(messages, err.Error())
			}
			assert.Equal(t, p.wantOK, results.OK(), "errors: %s", strings.Join(messages, "\n"))
			assert.Equal(t, p.wantRan, ranTest(results, subtestID))
			if !p.wantOK {
				assert.Contains(t, strings.Join(messages, "\n"), misroutedDiagnosticMessage)
			}
		})
	}
}
