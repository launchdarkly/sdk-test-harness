package mockld

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/launchdarkly/sdk-test-harness/v2/framework"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

// ListenerCallbackService is a mock HTTP server that receives flag change listener notifications
// POSTed by an SDK test service. Each registered listener should have its own instance so that
// notifications can be attributed to the correct listener in test assertions.
type ListenerCallbackService struct {
	payloadEndpoint *harness.MockEndpoint
	CallChannel     chan servicedef.ListenerNotification
}

// GetURL returns the callback URI to provide when registering a listener. The SDK test service
// will POST a ListenerNotification JSON body to this URL when the listener fires.
func (l *ListenerCallbackService) GetURL() string {
	return l.payloadEndpoint.BaseURL()
}

// Close shuts down the mock HTTP endpoint and releases its resources.
func (l *ListenerCallbackService) Close() {
	l.payloadEndpoint.Close()
}

// NewListenerCallbackService creates a ListenerCallbackService with a mock HTTP endpoint
// ready to receive notifications. Call Close() when done.
func NewListenerCallbackService(
	testHarness *harness.TestHarness,
	logger framework.Logger,
) *ListenerCallbackService {
	l := &ListenerCallbackService{
		CallChannel: make(chan servicedef.ListenerNotification),
	}

	endpointHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		bytes, err := io.ReadAll(req.Body)
		logger.Printf("Received listener notification: %s", string(bytes))
		if err != nil {
			logger.Printf("Could not read body from listener callback.")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var notification servicedef.ListenerNotification
		err = json.Unmarshal(bytes, &notification)
		if err != nil {
			logger.Printf("Could not unmarshal listener notification.")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		go func() {
			l.CallChannel <- notification
		}()

		w.WriteHeader(http.StatusOK)
	})

	l.payloadEndpoint = testHarness.NewMockEndpoint(
		endpointHandler, logger, harness.MockEndpointDescription("listener notification"))

	return l
}
