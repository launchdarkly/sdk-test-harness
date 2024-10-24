package harness

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"

	"github.com/stretchr/testify/assert"
)

func TestMockEndpointServesRequest(t *testing.T) {
	services := map[string]int{"http": 9998, "https": 9999}

	m := newMockEndpointsManager("testharness", services, framework.NullLogger())

	handler1 := httphelpers.HandlerWithStatus(200)
	e1 := m.newMockEndpoint(handler1, framework.NullLogger())

	handler2 := httphelpers.HandlerWithStatus(204)
	e2 := m.newMockEndpoint(handler2, framework.NullLogger())

	for service, port := range services {
		t.Run(service, func(t *testing.T) {
			m.SetService(service)

			assert.Equal(t, fmt.Sprintf("%s://testharness:%d/endpoints/1", service, port), e1.BaseURL())
			assert.Equal(t, fmt.Sprintf("%s://testharness:%d/endpoints/2", service, port), e2.BaseURL())

			rr1 := httptest.NewRecorder()
			r1, _ := http.NewRequest("GET", e1.BaseURL(), nil)
			m.serveHTTP(rr1, r1)
			assert.Equal(t, 200, rr1.Code)

			rr2 := httptest.NewRecorder()
			r2, _ := http.NewRequest("GET", e2.BaseURL(), nil)
			m.serveHTTP(rr2, r2)
			assert.Equal(t, 204, rr2.Code)
		})
	}
}

func TestMockEndpointReceivesSubpath(t *testing.T) {
	m := newMockEndpointsManager("testharness", map[string]int{"http": 9998}, framework.NullLogger())

	handler, requests := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(200))
	e := m.newMockEndpoint(handler, framework.NullLogger())
	assert.Equal(t, "http://testharness:9998/endpoints/1", e.BaseURL())

	for _, subpath := range []string{"", "/", "/sub/path"} {
		rr := httptest.NewRecorder()
		r, _ := http.NewRequest("GET", e.BaseURL()+subpath, nil)
		m.serveHTTP(rr, r)
		received := <-requests
		if subpath == "" {
			assert.Equal(t, "/", received.Request.URL.Path)
		} else {
			assert.Equal(t, subpath, received.Request.URL.Path)
		}
	}
}

func TestMockEndpointConnectionInfo(t *testing.T) {
	m := newMockEndpointsManager("testharness", map[string]int{"http": 9998}, framework.NullLogger())
	handler := httphelpers.HandlerWithStatus(200)
	e := m.newMockEndpoint(handler, framework.NullLogger())

	_, err := e.AwaitConnection(time.Millisecond * 50)
	assert.Error(t, err)

	rr1 := httptest.NewRecorder()
	r1, _ := http.NewRequest("GET", e.BaseURL(), nil)
	r1.Header.Add("header1", "value1")
	m.serveHTTP(rr1, r1)
	cxn1, err := e.AwaitConnection(time.Second)
	assert.NoError(t, err)
	assert.Equal(t, "GET", cxn1.Method)
	assert.Nil(t, cxn1.Body)
	assert.Equal(t, "value1", cxn1.Headers.Get("header1"))

	rr2 := httptest.NewRecorder()
	r2, _ := http.NewRequest("POST", e.BaseURL(), bytes.NewBuffer([]byte("content")))
	m.serveHTTP(rr2, r2)
	cxn2, err := e.AwaitConnection(time.Second)
	assert.NoError(t, err)
	assert.Equal(t, "POST", cxn2.Method)
	assert.Equal(t, []byte("content"), cxn2.Body)
}
