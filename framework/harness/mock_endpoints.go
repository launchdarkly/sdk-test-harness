package harness

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework"
	"github.com/launchdarkly/sdk-test-harness/framework/helpers"
)

const endpointPathPrefix = "/endpoints/"

// Somewhat arbitrary buffer size for the channel that we use as a queue for incoming connection
// information. If the channel is full, the HTTP request handler will *not* block; it will just
// discard the information.
const incomingConnectionChannelBufferSize = 10

type mockEndpointsManager struct {
	endpoints       map[string]*MockEndpoint
	lastEndpointID  int
	externalBaseURL string
	logger          framework.Logger
	lock            sync.Mutex
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

func newMockEndpointsManager(externalBaseURL string, logger framework.Logger) *mockEndpointsManager {
	return &mockEndpointsManager{
		endpoints:       make(map[string]*MockEndpoint),
		externalBaseURL: externalBaseURL,
		logger:          logger,
	}
}

func (m *mockEndpointsManager) newMockEndpoint(
	handler http.Handler,
	contextFn func(context.Context) context.Context,
	logger framework.Logger,
) *MockEndpoint {
	if logger == nil {
		logger = m.logger
	}
	e := &MockEndpoint{
		owner:     m,
		handler:   handler,
		contextFn: contextFn,
		newConns:  make(chan IncomingRequestInfo, incomingConnectionChannelBufferSize),
		logger:    logger,
	}
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
		data, err := ioutil.ReadAll(r.Body)
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
		transformedReq.Body = ioutil.NopCloser(bytes.NewBuffer(body))
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
	e.lock.Unlock()

	select { // non-blocking push
	case e.newConns <- *incoming:
		break
	default:
		m.logger.Printf("Incoming connection channel was full for %s", r.URL)
	}

	e.handler.ServeHTTP(w, transformedReq)

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
	return e.owner.externalBaseURL + e.basePath
}

// AwaitConnection waits for an incoming request to the endpoint.
func (e *MockEndpoint) AwaitConnection(timeout time.Duration) (IncomingRequestInfo, error) {
	maybeCxn := helpers.TryReceive(e.newConns, timeout)
	if maybeCxn.IsDefined() {
		return maybeCxn.Value(), nil
	}
	return IncomingRequestInfo{}, fmt.Errorf("timed out waiting for an incoming request to %s", e.description)
}

// RequireConnection waits for an incoming request to the endpoint, and causes the test to fail
// and terminate if it timed out.
func (e *MockEndpoint) RequireConnection(t helpers.TestContext, timeout time.Duration) IncomingRequestInfo {
	return helpers.RequireValueWithMessage(t, e.newConns, timeout, "timed out waiting for request to %s (%s)",
		e.description, e.basePath)
}

// RequireNoMoreConnections causes the test to fail and terminate if there is another incoming request
// within the timeout.
func (e *MockEndpoint) RequireNoMoreConnections(t helpers.TestContext, timeout time.Duration) {
	helpers.RequireNoMoreValuesWithMessage(t, e.newConns, timeout,
		"did not expect another request to %s (%s), but got one", e.description, e.basePath)
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
		e.owner.lock.Lock()
		delete(e.owner.endpoints, e.id)
		e.owner.lock.Unlock()

		e.lock.Lock()
		cancellers := e.cancels
		e.cancels = nil
		close(e.newConns)
		e.lock.Unlock()

		for _, cancel := range cancellers {
			(*cancel)()
		}
	})
}
