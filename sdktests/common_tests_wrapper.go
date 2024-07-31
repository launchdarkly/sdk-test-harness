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

		if p.WrapperName != "" {
			expectedHeaderValue = p.WrapperName
			if p.WrapperVersion != "" {
				expectedHeaderValue += "/" + p.WrapperVersion
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
		dataSource := NewSDKDataSource(t, nil)
		t.Run("no wrapper config", func(t *ldtest.T) {
			config := servicedef.SDKConfigWrapper{}
			events := NewSDKEventSink(t)
			client := NewSDKClient(t, c.baseSDKConfigurationPlus(
				withWrapper(config),
				dataSource,
				events)...)

			c.sendArbitraryEvent(t, client)
			client.FlushEvents(t)

			verifyRequestHeader(t, config, events.Endpoint())
		})

		t.Run("only wrapper name", func(t *ldtest.T) {
			config := servicedef.SDKConfigWrapper{WrapperName: "TestName"}
			events := NewSDKEventSink(t)
			client := NewSDKClient(t, c.baseSDKConfigurationPlus(
				withWrapper(config),
				dataSource,
				events)...)

			c.sendArbitraryEvent(t, client)
			client.FlushEvents(t)

			verifyRequestHeader(t, config, events.Endpoint())
		})

		t.Run("wrapper name and version", func(t *ldtest.T) {
			config := servicedef.SDKConfigWrapper{WrapperName: "TestName", WrapperVersion: "1.0.0"}
			events := NewSDKEventSink(t)
			client := NewSDKClient(t, c.baseSDKConfigurationPlus(
				withWrapper(config),
				dataSource,
				events)...)

			c.sendArbitraryEvent(t, client)
			client.FlushEvents(t)

			verifyRequestHeader(t, config, events.Endpoint())
		})

		t.Run("only wrapper version", func(t *ldtest.T) {
			config := servicedef.SDKConfigWrapper{WrapperVersion: "1.0.0"}
			events := NewSDKEventSink(t)
			client := NewSDKClient(t, c.baseSDKConfigurationPlus(
				withWrapper(config),
				dataSource,
				events)...)

			c.sendArbitraryEvent(t, client)
			client.FlushEvents(t)

			verifyRequestHeader(t, config, events.Endpoint())
		})
	})

	t.Run("stream requests", func(t *ldtest.T) {
		t.Run("wrapper name and version", func(t *ldtest.T) {
			config := servicedef.SDKConfigWrapper{WrapperName: "TestName", WrapperVersion: "1.0.0"}
			dataSource := NewSDKDataSource(t, nil, DataSourceOptionStreaming())
			configurers := c.baseSDKConfigurationPlus(
				withWrapper(config),
				dataSource)
			if c.isClientSide {
				// client-side SDKs in streaming mode may *also* need a polling data source
				configurers = append(configurers,
					NewSDKDataSource(t, nil, DataSourceOptionPolling()))
			}
			_ = NewSDKClient(t, configurers...)
			verifyRequestHeader(t, config, dataSource.Endpoint())
		})
	})

	t.Run("poll requests", func(t *ldtest.T) {
		// Currently server-side SDK test services do not support polling
		t.RequireCapability(servicedef.CapabilityClientSide)
		config := servicedef.SDKConfigWrapper{WrapperName: "TestName", WrapperVersion: "1.0.0"}
		t.Run("wrapper name and version", func(t *ldtest.T) {
			dataSource := NewSDKDataSource(t, nil, DataSourceOptionPolling())
			_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
				withWrapper(config),
				dataSource)...)
			verifyRequestHeader(t, config, dataSource.Endpoint())
		})
	})

}
