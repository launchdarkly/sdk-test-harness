package harness

import (
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
	"github.com/launchdarkly/sdk-test-harness/v2/serviceinfo"

	"github.com/launchdarkly/sdk-test-harness/v2/framework"
)

//go:embed certificate/cert.crt
var certificate []byte

//go:embed certificate/cert.key
var privateKey []byte

type certPaths struct {
	cert string
	key  string
}

func (c *certPaths) cleanup() {
	_ = os.Remove(c.cert)
	_ = os.Remove(c.key)
}

func exportCertificate() (*certPaths, error) {
	cert, err := os.CreateTemp("", "sdk-test-harness-cert*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp certificate file: %w", err)
	}
	if _, err := cert.Write(certificate); err != nil {
		return nil, fmt.Errorf("failed to write certificate to temp file: %w", err)
	}
	_ = cert.Close()

	key, err := os.CreateTemp("", "sdk-test-harness-cert-private-key*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp private key file: %w", err)
	}
	if _, err := key.Write(privateKey); err != nil {
		return nil, fmt.Errorf("failed to write private key to temp file: %w", err)
	}
	_ = key.Close()
	return &certPaths{cert: cert.Name(), key: key.Name()}, nil
}

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
	testServiceBaseURL string
	testServiceInfo    serviceinfo.TestServiceInfo
	mockEndpoints      *mockEndpointsManager
	mockHTTPSEndpoints *mockEndpointsManager
	logger             framework.Logger
	// Whether to use the mockEndpoints or mockHTTPSEndpoints when NewMockEndpoint is called.
	https bool
}

// SetHTTPS tells the test harness to generate HTTPS endpoints when NewMockEndpoint is called.
func (h *TestHarness) SetHTTPS(https bool) {
	h.https = https
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
	externalBaseHTTPSURL := fmt.Sprintf("https://%s:%d", testHarnessExternalHostname, testHarnessPort+1)

	h := &TestHarness{
		testServiceBaseURL: testServiceBaseURL,
		mockEndpoints:      newMockEndpointsManager(externalBaseURL, debugLogger),
		mockHTTPSEndpoints: newMockEndpointsManager(externalBaseHTTPSURL, debugLogger),
		logger:             debugLogger,
	}

	testServiceInfo, err := queryTestServiceInfo(testServiceBaseURL, statusQueryTimeout, startupOutput)
	if err != nil {
		return nil, err
	}
	h.testServiceInfo = testServiceInfo

	if err := startServer(testHarnessPort, http.HandlerFunc(h.serveHTTP)); err != nil {
		return nil, err
	}

	if testServiceInfo.Capabilities.HasAny(servicedef.CapabilityTLSSkipVerifyPeer, servicedef.CapabilityTLSVerifyPeer) {
		certInfo, err := exportCertificate()
		if err != nil {
			return nil, err
		}
		startHTTPSServer(testHarnessPort+1, certInfo, http.HandlerFunc(h.serveHTTPS))
	}

	return h, nil
}

// TestServiceInfo returns the initial status information received from the test service.
func (h *TestHarness) TestServiceInfo() serviceinfo.TestServiceInfo {
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
	if !h.https {
		return h.mockEndpoints.newMockEndpoint(handler, logger, options...)
	}
	return h.mockHTTPSEndpoints.newMockEndpoint(handler, logger, options...)
}

func (h *TestHarness) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "HEAD" {
		w.WriteHeader(200) // we use this to test whether our own listener is active yet
		return
	}
	h.mockEndpoints.serveHTTP(w, r)
}

func (h *TestHarness) serveHTTPS(w http.ResponseWriter, r *http.Request) {
	if r.Method == "HEAD" {
		w.WriteHeader(200) // we use this to test whether our own listener is active yet
		return
	}
	h.mockHTTPSEndpoints.serveHTTP(w, r)
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

func startHTTPSServer(port int, cert *certPaths, handler http.Handler) {
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
		defer cert.cleanup()
		if err := server.ListenAndServeTLS(cert.cert, cert.key); err != nil {
			panic(err)
		}
	}()
}
