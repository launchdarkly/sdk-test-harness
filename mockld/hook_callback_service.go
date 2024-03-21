package mockld

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/launchdarkly/sdk-test-harness/v2/framework"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
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

func NewHookCallbackService(
	testHarness *harness.TestHarness,
	logger framework.Logger,
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
			logger.Printf("Could not unmarshall hook payload.")
			w.WriteHeader(http.StatusBadRequest)
			return
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
