package sdktests

import (
	"time"

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
}

type Hooks struct {
	instances map[string]HookInstance
}

func NewHooks(
	testHarness *harness.TestHarness,
	logger framework.Logger,
	instances []string,
	data map[servicedef.HookStage]servicedef.SDKConfigEvaluationHookData,
) *Hooks {
	hooks := &Hooks{
		instances: make(map[string]HookInstance),
	}
	for _, instance := range instances {
		hooks.instances[instance] = HookInstance{
			name:        instance,
			hookService: mockld.NewHookCallbackService(testHarness, logger),
			data:        data,
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
