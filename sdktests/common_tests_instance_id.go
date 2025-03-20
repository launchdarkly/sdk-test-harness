package sdktests

import (
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/stretchr/testify/assert"
)

// CommonInstanceIDTests groups together event-related test methods that are shared between server-side and client-side.
type CommonInstanceIDTests struct {
	commonTestsBase
}

func NewCommonInstanceIDTests(t *ldtest.T, testName string, baseSDKConfigurers ...SDKConfigurer) CommonInstanceIDTests {
	return CommonInstanceIDTests{newCommonTestsBase(t, testName, baseSDKConfigurers...)}
}

func (c CommonInstanceIDTests) Run(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityInstanceID)

	verifyRequestHeader := func(t *ldtest.T, endpoint *harness.MockEndpoint) {
		request := endpoint.RequireConnection(t, time.Second)
		assert.NotEmpty(t, request.Headers.Get("X-LaunchDarkly-Instance-Id"))
	}

	t.Run("stream requests", func(t *ldtest.T) {
		dataSource := NewSDKDataSource(t, nil, DataSourceOptionStreaming())
		configurers := c.baseSDKConfigurationPlus(dataSource)
		if c.isClientSide {
			// client-side SDKs in streaming mode may *also* need a polling data source
			configurers = append(configurers,
				NewSDKDataSource(t, nil, DataSourceOptionPolling()))
		}
		_ = NewSDKClient(t, configurers...)
		verifyRequestHeader(t, dataSource.Endpoint())
	})

	t.Run("poll requests", func(t *ldtest.T) {
		t.Capabilities().HasAny(servicedef.CapabilityServerSidePolling, servicedef.CapabilityClientSide)

		dataSource := NewSDKDataSource(t, nil, DataSourceOptionPolling())
		_ = NewSDKClient(t, c.baseSDKConfigurationPlus(dataSource)...)
		verifyRequestHeader(t, dataSource.Endpoint())
	})

	t.Run("event posts", func(t *ldtest.T) {
		dataSource := NewSDKDataSource(t, nil)
		events := NewSDKEventSink(t)
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(
			dataSource,
			events)...)

		c.sendArbitraryEvent(t, client)
		client.FlushEvents(t)

		verifyRequestHeader(t, events.Endpoint())
	})
}

func (c CommonInstanceIDTests) RunPHP(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityInstanceID)

	verifyRequestHeader := func(t *ldtest.T, endpoint *harness.MockEndpoint) {
		request := endpoint.RequireConnection(t, time.Second)
		assert.NotEmpty(t, request.Headers.Get("X-LaunchDarkly-Instance-Id"))
	}

	t.Run("poll requests", func(t *ldtest.T) {
		dataSource := NewSDKDataSource(t, nil)
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSource)...)
		client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
			FlagKey:      "flag-key",
			Context:      o.Some(ldcontext.New("key")),
			ValueType:    servicedef.ValueTypeBool,
			DefaultValue: ldvalue.Bool(false),
			Detail:       false,
		})

		verifyRequestHeader(t, dataSource.Endpoint())
	})

	t.Run("event posts", func(t *ldtest.T) {
		dataSource := NewSDKDataSource(t, nil)
		events := NewSDKEventSink(t)
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(
			dataSource,
			events)...)

		c.sendArbitraryEvent(t, client)
		client.FlushEvents(t)

		verifyRequestHeader(t, events.Endpoint())
	})
}
