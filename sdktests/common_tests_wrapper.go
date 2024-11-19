package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/stretchr/testify/assert"
)

type CommonWrapperTests struct {
	commonTestsBase
}

func NewCommonWrapperTests(t *ldtest.T, testName string, baseSDKConfigurers ...SDKConfigurer) CommonWrapperTests {
	return CommonWrapperTests{newCommonTestsBase(t, testName, baseSDKConfigurers...)}
}

func (c CommonWrapperTests) Run(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityWrapper)

	verifyRequestHeader := func(t *ldtest.T, p servicedef.SDKConfigWrapper, endpoint *harness.MockEndpoint) {
		request := endpoint.RequireConnection(t, time.Second)
		expectedHeaderValue := ""

		if p.Name != "" {
			expectedHeaderValue = p.Name
			if p.Version != "" {
				expectedHeaderValue += "/" + p.Version
			}
		}

		if expectedHeaderValue == "" {
			assert.NotContains(t, request.Headers, "x-launchdarkly-wrapper")
		} else {
			assert.Equal(t, expectedHeaderValue, request.Headers.Get("x-launchdarkly-wrapper"))
		}
	}

	withWrapper := func(wrapper servicedef.SDKConfigWrapper) SDKConfigurer {
		return h.ConfigOptionFunc[servicedef.SDKConfigParams](func(config *servicedef.SDKConfigParams) error {
			config.Wrapper = o.Some(wrapper)
			return nil
		})
	}

	t.Run("event posts", func(t *ldtest.T) {
		dataSystem := NewSDKDataSystem(t, nil)
		t.Run("no wrapper config", func(t *ldtest.T) {
			config := servicedef.SDKConfigWrapper{}
			events := NewSDKEventSink(t)
			client := NewSDKClient(t, c.baseSDKConfigurationPlus(
				withWrapper(config),
				dataSystem,
				events)...)

			c.sendArbitraryEvent(t, client)
			client.FlushEvents(t)

			verifyRequestHeader(t, config, events.Endpoint())
		})

		t.Run("only wrapper name", func(t *ldtest.T) {
			config := servicedef.SDKConfigWrapper{Name: "TestName"}
			events := NewSDKEventSink(t)
			client := NewSDKClient(t, c.baseSDKConfigurationPlus(
				withWrapper(config),
				dataSystem,
				events)...)

			c.sendArbitraryEvent(t, client)
			client.FlushEvents(t)

			verifyRequestHeader(t, config, events.Endpoint())
		})

		t.Run("wrapper name and version", func(t *ldtest.T) {
			config := servicedef.SDKConfigWrapper{Name: "TestName", Version: "1.0.0"}
			events := NewSDKEventSink(t)
			client := NewSDKClient(t, c.baseSDKConfigurationPlus(
				withWrapper(config),
				dataSystem,
				events)...)

			c.sendArbitraryEvent(t, client)
			client.FlushEvents(t)

			verifyRequestHeader(t, config, events.Endpoint())
		})

		t.Run("only wrapper version", func(t *ldtest.T) {
			config := servicedef.SDKConfigWrapper{Version: "1.0.0"}
			events := NewSDKEventSink(t)
			client := NewSDKClient(t, c.baseSDKConfigurationPlus(
				withWrapper(config),
				dataSystem,
				events)...)

			c.sendArbitraryEvent(t, client)
			client.FlushEvents(t)

			verifyRequestHeader(t, config, events.Endpoint())
		})
	})

	t.Run("stream requests", func(t *ldtest.T) {
		t.Run("wrapper name and version", func(t *ldtest.T) {
			config := servicedef.SDKConfigWrapper{Name: "TestName", Version: "1.0.0"}
			dataSystem := NewSDKDataSystem(t, nil, DataSystemOptionStreaming())
			configurers := c.baseSDKConfigurationPlus(
				withWrapper(config),
				dataSystem)
			if c.isClientSide {
				// client-side SDKs in streaming mode may *also* need a polling data source
				configurers = append(configurers,
					NewSDKDataSystem(t, nil, DataSystemOptionPolling()))
			}
			_ = NewSDKClient(t, configurers...)
			verifyRequestHeader(t, config, dataSystem.Synchronizers.primary.Endpoint())
		})
	})

	t.Run("poll requests", func(t *ldtest.T) {
		config := servicedef.SDKConfigWrapper{Name: "TestName", Version: "1.0.0"}
		t.Run("wrapper name and version", func(t *ldtest.T) {
			dataSystem := NewSDKDataSystem(t, nil, DataSystemOptionPolling())
			_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
				withWrapper(config),
				dataSystem)...)
			verifyRequestHeader(t, config, dataSystem.Synchronizers.primary.Endpoint())
		})
	})
}
