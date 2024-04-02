package sdktests

import (
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/sdk-test-harness/v2/framework"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

const hookReceiveTimeout = time.Second * 5

type HookInstance struct {
	name        string
	hookService *mockld.HookCallbackService
	data        map[servicedef.HookStage]servicedef.SDKConfigEvaluationHookData
	errors      map[servicedef.HookStage]o.Maybe[string]
}

type Hooks struct {
	instances map[string]HookInstance
}

func NewHooks(
	testHarness *harness.TestHarness,
	logger framework.Logger,
	instances []string,
	data map[servicedef.HookStage]servicedef.SDKConfigEvaluationHookData,
	errors map[servicedef.HookStage]o.Maybe[string],
) *Hooks {
	hooks := &Hooks{
		instances: make(map[string]HookInstance),
	}
	for _, instance := range instances {
		hooks.instances[instance] = HookInstance{
			name:        instance,
			hookService: mockld.NewHookCallbackService(testHarness, logger),
			data:        data,
			errors:      errors,
		}
	}

	return hooks
}

func (h *Hooks) Configure(config *servicedef.SDKConfigParams) error {
	hookConfig := config.Hooks.Value()
	for _, instance := range h.instances {
		hookConfig.Hooks = append(hookConfig.Hooks, servicedef.SDKConfigHookInstance{
			Name:        instance.name,
			CallbackURI: instance.hookService.GetURL(),
			Data:        instance.data,
			Errors:      instance.errors,
		})
	}
	config.Hooks = o.Some(hookConfig)
	return nil
}

func (h *Hooks) Close() {
	for _, instance := range h.instances {
		instance.hookService.Close()
	}
}

func (h *Hooks) ExpectCall(t *ldtest.T, hookName string,
	matcher func(payload servicedef.HookExecutionPayload) bool) {
	for {
		maybeValue := helpers.TryReceive(h.instances[hookName].hookService.CallChannel, hookReceiveTimeout)
		if !maybeValue.IsDefined() {
			t.Errorf("Timed out trying to receive hook execution data")
			t.FailNow()
			break
		}
		payload := maybeValue.Value()
		if matcher(payload) {
			break
		}
	}
}

// ExpectAtLeastOneCallForEachHook waits for a single call from N hooks. If there are fewer calls recorded,
// the test will fail. However, this helper cannot detect if there were more calls waiting to be recorded.
func (h *Hooks) ExpectAtLeastOneCallForEachHook(t *ldtest.T, hookNames []string) []servicedef.HookExecutionPayload {
	out := make(chan o.Maybe[servicedef.HookExecutionPayload])

	totalCalls := len(hookNames)

	for _, hookName := range hookNames {
		go func(name string) {
			out <- helpers.TryReceive(h.instances[name].hookService.CallChannel, hookReceiveTimeout)
		}(hookName)
	}

	payloads := make([]servicedef.HookExecutionPayload, totalCalls)
	for i := 0; i < totalCalls; i++ {
		if val := <-out; val.IsDefined() {
			payloads = append(payloads, val.Value())
		}
	}

	assert.Len(t, payloads, totalCalls, "Expected %d hook calls, got %d", totalCalls, len(payloads))

	return payloads
}
