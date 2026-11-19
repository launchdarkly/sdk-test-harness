package sdktests

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v3/framework"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeMobileSDKBehavior selects where the fake test service sends its diagnostic-init event.
type fakeMobileSDKBehavior int

const (
	// The diagnostic-init event goes to the diagnostic events route.
	fakeSDKRoutesDiagnosticCorrectly fakeMobileSDKBehavior = iota
	// The diagnostic-init event goes to the analytics events route as one JSON object.
	fakeSDKPostsDiagnosticToAnalyticsRoute
	// The diagnostic-init event goes to the diagnostic events route. A second diagnostic event is
	// an item of the analytics payload.
	fakeSDKPutsDiagnosticInAnalyticsPayload
)

// fakeMobileSDKService is a minimal SDK test service for a mobile SDK. It does not run an SDK. It
// sends the event posts that an SDK with the given behavior sends.
type fakeMobileSDKService struct {
	behavior      fakeMobileSDKBehavior
	capabilities  []string
	eventsBaseURI string
	lock          sync.Mutex
}

func (f *fakeMobileSDKService) setEventsBaseURI(uri string) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.eventsBaseURI = strings.TrimSuffix(uri, "/")
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
		f.setEventsBaseURI(eventsConfig.BaseURI)
		if eventsConfig.EnableDiagnostics {
			if f.behavior == fakeSDKPostsDiagnosticToAnalyticsRoute {
				f.post("/mobile", `{"kind":"diagnostic-init"}`)
			} else {
				f.post("/mobile/events/diagnostic", `{"kind":"diagnostic-init"}`)
			}
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
			if f.behavior == fakeSDKPutsDiagnosticInAnalyticsPayload {
				f.post("/mobile/events/bulk", `[{"kind":"identify"},{"kind":"custom"},{"kind":"diagnostic"}]`)
			} else {
				f.post("/mobile/events/bulk", `[{"kind":"identify"},{"kind":"custom"}]`)
			}
		}
		w.WriteHeader(http.StatusAccepted)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// runDiagnosticEventTestsAgainstFakeSDK runs CommonEventTests.DiagnosticEvents against a fake test
// service, and returns the results and the messages of all the test failures.
func runDiagnosticEventTestsAgainstFakeSDK(
	t *testing.T,
	behavior fakeMobileSDKBehavior,
	capabilities ...string,
) (ldtest.Results, string) {
	service := &fakeMobileSDKService{behavior: behavior, capabilities: capabilities}
	server := httptest.NewServer(service)
	defer server.Close()

	// Get a free port for the callback listener of the test harness.
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())

	testHarness, err := harness.NewTestHarness(server.URL, "localhost", port, false, time.Second*5,
		framework.NullLogger(), io.Discard)
	require.NoError(t, err)

	results := ldtest.Run(
		ldtest.TestConfiguration{
			Capabilities: testHarness.TestServiceInfo().Capabilities,
			Context:      SDKTestContext{harness: testHarness, sdkKind: mockld.MobileSDK},
		},
		func(t *ldtest.T) {
			t.Run("diagnostic events", func(t *ldtest.T) {
				NewCommonEventTests(t, "runDiagnosticEventTestsAgainstFakeSDK").DiagnosticEvents(t)
			})
		})

	var messages []string
	for _, failure := range results.Failures {
		for _, e := range failure.Errors {
			messages = append(messages, e.Error())
		}
	}
	return results, strings.Join(messages, "\n")
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
	allCapabilities := []string{
		servicedef.CapabilityClientSide,
		servicedef.CapabilityMobile,
		servicedef.CapabilityDiagnosticEvents,
	}

	t.Run("passes for an SDK that uses the diagnostic route", func(t *testing.T) {
		results, messages := runDiagnosticEventTestsAgainstFakeSDK(t,
			fakeSDKRoutesDiagnosticCorrectly, allCapabilities...)
		assert.True(t, results.OK(), "failures: %s", messages)
		assert.True(t, ranTest(results, subtestID), "subtest did not run")
	})

	t.Run("fails for an SDK that posts the diagnostic object to the analytics route", func(t *testing.T) {
		results, messages := runDiagnosticEventTestsAgainstFakeSDK(t,
			fakeSDKPostsDiagnosticToAnalyticsRoute, allCapabilities...)
		assert.False(t, results.OK())
		assert.Contains(t, messages, "received a diagnostic event on the analytics events route")
	})

	t.Run("fails for an SDK that puts a diagnostic event in the analytics payload", func(t *testing.T) {
		results, messages := runDiagnosticEventTestsAgainstFakeSDK(t,
			fakeSDKPutsDiagnosticInAnalyticsPayload, allCapabilities...)
		assert.False(t, results.OK())
		assert.Contains(t, messages, "received a diagnostic event on the analytics events route")
	})

	t.Run("does not run without the capability", func(t *testing.T) {
		results, messages := runDiagnosticEventTestsAgainstFakeSDK(t,
			fakeSDKPostsDiagnosticToAnalyticsRoute, servicedef.CapabilityClientSide, servicedef.CapabilityMobile)
		assert.True(t, results.OK(), "failures: %s", messages)
		assert.False(t, ranTest(results, subtestID), "subtest ran without the capability")
	})
}
