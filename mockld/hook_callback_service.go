package mockld

import (
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"

	"github.com/launchdarkly/sdk-test-harness/v3/framework"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"
)

type HookCallbackService struct {
	payloadEndpoint *harness.MockEndpoint
	CallChannel     chan servicedef.HookExecutionPayload
	stopChannel     chan struct{}
}

func (h *HookCallbackService) GetURL() string {
	return h.payloadEndpoint.BaseURL()
}

func (h *HookCallbackService) Close() {
	h.payloadEndpoint.Close()
}

// NewHookCallbackService creates an HTTP endpoint that records incoming hook
// callbacks. If sequence is non-nil it is incremented per received call and
// stamped onto the payload so tests can establish ordering across hooks that
// share the same counter.
func NewHookCallbackService(
	testHarness *harness.TestHarness,
	logger framework.Logger,
	sequence *atomic.Int64,
) *HookCallbackService {
	h := &HookCallbackService{
		CallChannel: make(chan servicedef.HookExecutionPayload),
		stopChannel: make(chan struct{}),
	}

	endpointHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		bytes, err := io.ReadAll(req.Body)
		logger.Printf("Received from hook: %s", string(bytes))
		if err != nil {
			logger.Printf("Could not read body from hook.")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var response servicedef.HookExecutionPayload
		err = json.Unmarshal(bytes, &response)
		if err != nil {
			logger.Printf("Could not unmarshal hook payload.")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if sequence != nil {
			response.Sequence = sequence.Add(1)
		}

		go func() {
			h.CallChannel <- response
		}()

		w.WriteHeader(http.StatusOK)
	})

	h.payloadEndpoint = testHarness.NewMockEndpoint(
		endpointHandler, logger, harness.MockEndpointDescription("hook payload"))

	return h
}
