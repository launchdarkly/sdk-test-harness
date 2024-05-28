package harness

import (
	"bytes"
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

//go:embed certificate/leaf_public.pem
var certificate []byte

//go:embed certificate/leaf_private.pem
var privateKey []byte

//go:embed certificate/ca_public.pem
var caCertificate []byte

type certPaths struct {
	cert string
	key  string
	ca   string
}

func (c *certPaths) cleanup() {
	_ = os.Remove(c.cert)
	_ = os.Remove(c.key)
	_ = os.Remove(c.ca)
}

func makeTempFile(pattern string, data []byte) (string, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint: errcheck
	if _, err := f.Write(data); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func exportCertChain() (*certPaths, error) {
	chain := bytes.NewBuffer(certificate)
	chain.Write(caCertificate)

	cert, err := makeTempFile("sdk-test-harness-cert*", chain.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to create temp certificate file: %w", err)
	}

	key, err := makeTempFile("sdk-test-harness-cert-private-key*", privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp private key file: %w", err)
	}

	ca, err := makeTempFile("sdk-test-harness-ca-cert*", caCertificate)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp ca certificate file: %w", err)
	}
	return &certPaths{cert: cert, key: key, ca: ca}, nil
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
	logger             framework.Logger
	caCertPath         string
}

// SetService tells the endpoint manager which protocol should be used when BaseURL() is called on a MockEndpoint.
// Reaching into this  object is unfortunate, but since this is essentially a global variable from each
// tests' perspective, this is the only way to modify it.
// The service string should be one of 'http' or 'https'.
func (h *TestHarness) SetService(service string) {
	h.mockEndpoints.SetService(service)
}

// CertificateAuthorityPath returns the path to CA cert used by the test harness when establishing a TLS
// connection with the SDK under test.
func (h *TestHarness) CertificateAuthorityPath() string {
	return h.caCertPath
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

	h := &TestHarness{
		testServiceBaseURL: testServiceBaseURL,
		mockEndpoints: newMockEndpointsManager(
			testHarnessExternalHostname,
			map[string]int{"http": testHarnessPort, "https": testHarnessPort + 1},
			debugLogger),
		logger: debugLogger,
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
		certInfo, err := exportCertChain()
		if err != nil {
			return nil, err
		}
		h.caCertPath = certInfo.ca
		startHTTPSServer(testHarnessPort+1, certInfo, http.HandlerFunc(h.serveHTTP))
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
