package harness

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework"
)

const httpListenerTimeout = time.Second * 10

var excludeHTTPServerLogPatterns = []*regexp.Regexp{ //nolint:gochecknoglobals
	// httphelpers.BrokenConnectionHandler may be used by tests to force a connection failure on an HTTP
	// request. The Go HTTP framework provides no way to do this other than by triggering a panic, which
	// will be caught and logged by the http.Server; we don't want such logging to appear in test output.
	regexp.MustCompile("panic.*httphelpers.BrokenConnectionHandler"),
}

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

	if err = startServer(testHarnessPort, http.HandlerFunc(h.serveHTTP)); err != nil {
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
		// Provide a custom error logger so we can filter out some irrelevant output from the http.Server,
		// while still letting it log any other unusual conditions that might be related to a test failure.
		// Note that it is OK to filter messages with a custom Writer in this way because log.Printf and
		// log.Println always make one individual Write call for each log item-- messages aren't batched.
		ErrorLog: log.New(newFilteredWriter(os.Stderr, excludeHTTPServerLogPatterns),
			"TestHarnessHTTPServer: ", log.LstdFlags),
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
