package harness

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework"
)

const httpListenerTimeout = time.Second * 10

// TestHarness is the main component that manages communication with test services.
//
// It always communicates with a single test service, which it verifies is alive on startup. It
// can then create any number of test service entities within the test service (NewTestServiceEntity)
// and any number of callback endpoints for the test service to interact with (NewMockEndpoint).
//
// It contains no domain-specific test logic, but only provides a general mechanism for test suites
// to build on.
type TestHarness struct {
	testServiceBaseURL         string
	testHarnessExternalBaseURL string
	testServiceInfo            TestServiceInfo
	mockEndpoints              *mockEndpointsManager
	logger                     framework.Logger
}

// NewTestHarness creates a TestHarness instance, and verifies that the test service
// is responding by querying its status resource. It also starts an HTTP listener
// on the specified port to receive callback requests.
func NewTestHarness(
	testServiceBaseURL string,
	testHarnessExternalHostname string,
	testHarnessPort int,
	statusQueryTimeout time.Duration,
	debugLogger framework.Logger,
	startupOutput io.Writer,
) (*TestHarness, error) {
	if debugLogger == nil {
		debugLogger = framework.NullLogger()
	}

	externalBaseURL := fmt.Sprintf("http://%s:%d", testHarnessExternalHostname, testHarnessPort)

	h := &TestHarness{
		testServiceBaseURL:         testServiceBaseURL,
		testHarnessExternalBaseURL: externalBaseURL,
		mockEndpoints:              newMockEndpointsManager(externalBaseURL, debugLogger),
		logger:                     debugLogger,
	}

	testServiceInfo, err := queryTestServiceInfo(testServiceBaseURL, statusQueryTimeout, startupOutput)
	if err != nil {
		return nil, err
	}
	h.testServiceInfo = testServiceInfo

	if err := startServer(testHarnessPort, http.HandlerFunc(h.serveHTTP)); err != nil {
		return nil, err
	}

	return h, nil
}

// TestServiceInfo returns the initial status information received from the test service.
func (h *TestHarness) TestServiceInfo() TestServiceInfo {
	return h.testServiceInfo
}

// NewMockEndpoint adds a new endpoint that can receive requests.
//
// The specified handler will be called for all incoming requests to the endpoint's
// base URL or any subpath of it. For instance, if the generated base URL (as reported
// by MockEndpoint.BaseURL()) is http://localhost:8111/endpoints/3, then it can also
// receive requests to http://localhost:8111/endpoints/3/some/subpath.
//
// When the handler is called, the test harness rewrites the request URL first so that
// the handler sees only the subpath. It also attaches a Context to the request whose
// Done channel will be closed if Close is called on the endpoint.
func (h *TestHarness) NewMockEndpoint(
	handler http.Handler,
	logger framework.Logger,
	options ...MockEndpointOption,
) *MockEndpoint {
	if logger == nil {
		logger = h.logger
	}
	return h.mockEndpoints.newMockEndpoint(handler, logger, options...)
}

func (h *TestHarness) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "HEAD" {
		w.WriteHeader(200) // we use this to test whether our own listener is active yet
		return
	}
	h.mockEndpoints.serveHTTP(w, r)
}

func startServer(port int, handler http.Handler) error {
	server := &http.Server{
		Addr: fmt.Sprintf(":%d", port),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "HEAD" {
				w.WriteHeader(200)
				return
			}
			handler.ServeHTTP(w, r)
		}),
		ReadHeaderTimeout: 10 * time.Second, // arbitrary but non-infinite timeout to avoid Slowloris Attack
	}
	go func() {
		if err := server.ListenAndServe(); err != nil {
			panic(err)
		}
	}()

	// Wait till the server is definitely listening for requests before we run any tests
	deadline := time.NewTimer(httpListenerTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond * 10)
	defer ticker.Stop()
	for {
		select {
		case <-deadline.C:
			return fmt.Errorf("could not detect own listener at %s", server.Addr)
		case <-ticker.C:
			_, _, err := doRequest("HEAD", fmt.Sprintf("http://localhost:%d", port), nil)
			if err == nil {
				return nil
			}
		}
	}
}

func startHTTPSServer(port int, handler http.Handler) {
	server := &http.Server{
		Addr: fmt.Sprintf(":%d", port),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "HEAD" {
				w.WriteHeader(200)
				return
			}
			handler.ServeHTTP(w, r)
		}),
		ReadHeaderTimeout: 10 * time.Second, // arbitrary but non-infinite timeout to avoid Slowloris Attack,
	}
	go func() {
		if err := server.ListenAndServeTLS("certificate/cert.crt", "certificate/cert.key"); err != nil {
			panic(err)
		}
	}()
}
