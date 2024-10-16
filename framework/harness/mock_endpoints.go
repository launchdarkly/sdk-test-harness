package harness

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
)

const endpointPathPrefix = "/endpoints/"

// Somewhat arbitrary buffer size for the channel that we use as a queue for incoming connection
// information. If the channel is full, the HTTP request handler will *not* block; it will just
// discard the information.
const incomingConnectionChannelBufferSize = 10

type mockEndpointsManager struct {
	endpoints      map[string]*MockEndpoint
	lastEndpointID int
	host           string
	// Maps a service to its port, e.g. http -> 9998, https -> 9999
	services map[string]int
	// Used to select the correct port when BaseURL() is called by test components.
	service string
	logger  framework.Logger
	lock    sync.Mutex
}

// MockEndpoint represents an endpoint that can receive requests.
type MockEndpoint struct {
	owner       *mockEndpointsManager
	id          string
	description string
	basePath    string
	handler     http.Handler
	contextFn   func(context.Context) context.Context
	newConns    chan IncomingRequestInfo
	activeConn  *IncomingRequestInfo
	cancels     []*context.CancelFunc
	logger      framework.Logger
	lock        sync.Mutex
	closing     sync.Once
}

// MockEndpointOption is the interface for options to NewMockEndpoint.
type MockEndpointOption helpers.ConfigOption[MockEndpoint]

type mockEndpointOptionContextFn struct {
	contextFn func(context.Context) context.Context
}

func (o mockEndpointOptionContextFn) Configure(m *MockEndpoint) error {
	m.contextFn = o.contextFn
	return nil
}

// MockEndpointContextFn is an option to set a context transformation function for the endpoint that will be
// called for each request. This can be used if the test logic managing the endpoint needs to be able to
// share some kind of state between the endpoint's HTTP handler and code elsewhere, by embedding a value in
// the context.
func MockEndpointContextFn(fn func(context.Context) context.Context) MockEndpointOption {
	return mockEndpointOptionContextFn{fn}
}

type mockEndpointOptionDescription struct {
	description string
}

func (o mockEndpointOptionDescription) Configure(m *MockEndpoint) error {
	m.description = o.description
	return nil
}

// MockEndpointDescription is an option to set a descriptive name for the endpoint, such as "streaming service".
func MockEndpointDescription(description string) MockEndpointOption {
	return mockEndpointOptionDescription{description}
}

// IncomingRequestInfo contains information about an HTTP request sent by the test service
// to one of the mock endpoints.
type IncomingRequestInfo struct {
	Headers http.Header
	Method  string
	URL     url.URL
	Body    []byte
	Context context.Context
	Cancel  context.CancelFunc
}

func newMockEndpointsManager(host string, services map[string]int, logger framework.Logger) *mockEndpointsManager {
	return &mockEndpointsManager{
		endpoints: make(map[string]*MockEndpoint),
		host:      host,
		services:  services,
		service:   "http",
		logger:    logger,
	}
}

func (m *mockEndpointsManager) SetService(service string) {
	_, ok := m.services[service]
	if !ok {
		panic("programmer error: cannot set service to " + service + ", it has no port mapping")
	}
	m.service = service
}
func (m *mockEndpointsManager) BaseURL() string {
	port, ok := m.services[m.service]
	if !ok {
		panic("programmer error: service " + m.service + " has no port mapping")
	}
	return m.service + "://" + m.host + ":" + fmt.Sprintf("%d", port)
}

func (m *mockEndpointsManager) newMockEndpoint(
	handler http.Handler,
	logger framework.Logger,
	options ...MockEndpointOption,
) *MockEndpoint {
	if logger == nil {
		logger = m.logger
	}
	e := &MockEndpoint{
		owner:    m,
		handler:  handler,
		newConns: make(chan IncomingRequestInfo, incomingConnectionChannelBufferSize),
		logger:   logger,
	}
	_ = helpers.ApplyOptions(e, options...)
	m.lock.Lock()
	m.lastEndpointID++
	e.id = strconv.Itoa(m.lastEndpointID)
	e.basePath = endpointPathPrefix + e.id
	m.endpoints[e.id] = e
	m.lock.Unlock()

	return e
}

func (m *mockEndpointsManager) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, endpointPathPrefix) {
		m.logger.Printf("Received request for unrecognized URL path %s", r.URL.Path)
		w.WriteHeader(404)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, endpointPathPrefix)
	var endpointID string
	slashPos := strings.Index(path, "/")
	if slashPos >= 0 {
		endpointID = path[0:slashPos]
		path = path[slashPos:]
	} else {
		endpointID = path
		path = "/"
	}

	m.lock.Lock()
	e := m.endpoints[endpointID]
	m.lock.Unlock()
	if e == nil {
		m.logger.Printf("Received request for unrecognized endpoint %s", r.URL.Path)
		w.WriteHeader(404)
		return
	}

	var body []byte
	if r.Body != nil {
		data, err := io.ReadAll(r.Body)
		_ = r.Body.Close()
		if err != nil {
			m.logger.Printf("Unexpected error trying to read request body: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		body = data
	}

	ctx, canceller := context.WithCancel(r.Context())
	if e.contextFn != nil {
		ctx = e.contextFn(ctx)
	}
	transformedReq := r.WithContext(ctx)
	url := *r.URL
	url.Path = path
	transformedReq.URL = &url
	if body != nil {
		transformedReq.Body = io.NopCloser(bytes.NewBuffer(body))
	}

	incoming := &IncomingRequestInfo{
		Headers: r.Header,
		Method:  r.Method,
		URL:     url,
		Body:    body,
		Context: ctx,
		Cancel:  canceller,
	}

	e.lock.Lock()
	e.activeConn = incoming
	cancellerPtr := &canceller
	e.cancels = append(e.cancels, cancellerPtr)
	newConns := e.newConns
	e.lock.Unlock()

	if newConns == nil {
		// the endpoint is already closed
		m.logger.Printf("Received request to already-closed endpoint %s", r.URL)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	select { // non-blocking push
	case newConns <- *incoming:
		break
	default:
		m.logger.Printf("Incoming connection channel was full for %s", r.URL)
	}

	wrappedWriter := wrappedResponseWriter{w: w}
	e.handler.ServeHTTP(&wrappedWriter, transformedReq)

	switch wrappedWriter.status {
	case http.StatusNotFound:
		e.logger.Printf("Endpoint %q (%s) received %s request for unrecognized path %s", e.description, e.basePath,
			r.Method, path)
	case http.StatusMethodNotAllowed:
		e.logger.Printf("Endpoint %q (%s) received request with unsupported %s method for path %s", e.description,
			e.basePath, r.Method, path)
	}

	e.lock.Lock()
	for i, c := range e.cancels {
		if c == cancellerPtr { // can't compare functions with ==, but can compare pointers
			e.cancels = append(e.cancels[:i], e.cancels[i+1:]...)
			break
		}
	}
	e.lock.Unlock()
}

// BaseURL returns the base path of the mock endpoint.
func (e *MockEndpoint) BaseURL() string {
	joined, err := url.JoinPath(e.owner.BaseURL(), e.basePath)
	if err != nil {
		panic("invalid sdk-test-harness base URL: " + err.Error())
	}
	return joined
}

// AwaitConnection waits for an incoming request to the endpoint.
func (e *MockEndpoint) AwaitConnection(timeout time.Duration) (IncomingRequestInfo, error) {
	maybeCxn := helpers.TryReceive(e.newConns, timeout)
	if maybeCxn.IsDefined() {
		return maybeCxn.Value(), nil
	}
	return IncomingRequestInfo{}, fmt.Errorf("timed out waiting for an incoming request to %q (%s)", e.description,
		e.basePath)
}

// RequireConnection waits for an incoming request to the endpoint, and causes the test to fail
// and terminate if it timed out.
func (e *MockEndpoint) RequireConnection(t helpers.TestContext, timeout time.Duration) IncomingRequestInfo {
	return helpers.RequireValueWithMessage(t, e.newConns, timeout, "timed out waiting for request to %q (%s)",
		e.description, e.basePath)
}

// RequireNoMoreConnections causes the test to fail and terminate if there is another incoming request
// within the timeout.
func (e *MockEndpoint) RequireNoMoreConnections(t helpers.TestContext, timeout time.Duration) {
	helpers.RequireNoMoreValuesWithMessage(t, e.newConns, timeout,
		"did not expect another request to %q (%s), but got one", e.description, e.basePath)
}

func (e *MockEndpoint) ActiveConnection() *IncomingRequestInfo {
	e.lock.Lock()
	defer e.lock.Unlock()
	return e.activeConn
}

// Close unregisters the endpoint. Any subsequent requests to it will receive 404 errors.
// It also cancels the Context for every active request to that endpoint.
func (e *MockEndpoint) Close() {
	e.closing.Do(func() {
		e.logger.Printf("Closing endpoint %q (%s)", e.description, e.basePath)
		e.owner.lock.Lock()
		delete(e.owner.endpoints, e.id)
		e.owner.lock.Unlock()

		e.lock.Lock()
		cancellers := e.cancels
		e.cancels = nil
		close(e.newConns)
		e.newConns = nil
		e.lock.Unlock()

		for _, cancel := range cancellers {
			(*cancel)()
		}
	})
}

// wrappedResponseWriter is a way for us to monitor the status that is written to a ResponseWriter,
// so we can add some debug logging for 404 and 405 statuses.
type wrappedResponseWriter struct {
	w      http.ResponseWriter
	status int
}

func (ww *wrappedResponseWriter) Header() http.Header { return ww.w.Header() }

func (ww *wrappedResponseWriter) WriteHeader(status int) {
	ww.status = status
	ww.w.WriteHeader(status)
}

func (ww *wrappedResponseWriter) Write(data []byte) (int, error) { return ww.w.Write(data) }

// Flush is not a http.ResponseWriter interface method, but is implemented by the real ResponseWriter
// implementation in http.Server, so we need to be able to delegate to it.
func (ww *wrappedResponseWriter) Flush() {
	if f, ok := ww.w.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack is not a http.ResponseWriter interface method, but is implemented by the real ResponseWriter
// implementation in http.Server, so we need to be able to delegate to it. If we don't implement this,
// then the mechanism used by httphelpers.BrokenConnectionHandler to break a connection doesn't work
// quite right and will cause a panic stacktrace.
func (ww *wrappedResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := ww.w.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("Hijack was called when the underlying ResponseWriter did not support it")
}
