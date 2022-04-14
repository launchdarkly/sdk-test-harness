package sdktests

import (
	"errors"

	"github.com/launchdarkly/sdk-test-harness/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"

	"github.com/stretchr/testify/require"
)

// SDKConfigurer is an interface for objects that can modify the configuration for StartSDKClient.
// It is implemented by types such as SDKDataSource.
type SDKConfigurer interface {
	ApplyConfiguration(*servicedef.SDKConfigParams)
}

type sdkConfigurerFunc func(*servicedef.SDKConfigParams)

func (f sdkConfigurerFunc) ApplyConfiguration(configOut *servicedef.SDKConfigParams) { f(configOut) }

// WithConfig is used with StartSDKClient to specify a non-default SDK configuration. Use this
// before any other SDKConfigurers or it will overwrite their effects.
func WithConfig(config servicedef.SDKConfigParams) SDKConfigurer {
	return sdkConfigurerFunc(func(configOut *servicedef.SDKConfigParams) {
		*configOut = config
	})
}

// WithClientSideConfig is used with StartSDKClient to specify a non-default client-side SDK
// configuration.
func WithClientSideConfig(clientSideConfig servicedef.SDKConfigClientSideParams) SDKConfigurer {
	return sdkConfigurerFunc(func(configOut *servicedef.SDKConfigParams) {
		configOut.ClientSide = o.Some(clientSideConfig)
	})
}

// WithEventsConfig is used with StartSDKClient to specify a non-default events configuration.
func WithEventsConfig(eventsConfig servicedef.SDKConfigEventParams) SDKConfigurer {
	return sdkConfigurerFunc(func(configOut *servicedef.SDKConfigParams) {
		configOut.Events = o.Some(eventsConfig)
	})
}

// WithServiceEndpointsConfig is used with StartSDKClient to specify non-default service endpoints.
// This will only work if the test service has the "service-endpoints" capability.
func WithServiceEndpointsConfig(endpointsConfig servicedef.SDKConfigServiceEndpointsParams) SDKConfigurer {
	return sdkConfigurerFunc(func(configOut *servicedef.SDKConfigParams) {
		configOut.ServiceEndpoints = o.Some(endpointsConfig)
	})
}

// WithStreamingConfig is used with StartSDKClient to specify a non-default streaming configuration.
func WithStreamingConfig(streamingConfig servicedef.SDKConfigStreamingParams) SDKConfigurer {
	return sdkConfigurerFunc(func(configOut *servicedef.SDKConfigParams) {
		configOut.Streaming = o.Some(streamingConfig)
	})
}

// SDKClient represents an SDK client instance in the test service which can be controlled by test logic.
type SDKClient struct {
	sdkConfig       servicedef.SDKConfigParams
	sdkClientEntity *harness.TestServiceEntity
}

// NewSDKClient tells the test service to create an SDK client instance.
//
// The first parameter should be the current test scope. Any error in creating the client will cause the
// test to fail and terminate immediately. Debug output related to the client will be attached to this
// test scope.
//
// You must always specify at least one SDKConfigurer to customize the SDK configuration, since a default
// SDK configuration would only connect to LaunchDarkly which is normally not what we want. Test fixture
// components such as SDKDataSource implement this interface so that they can insert the appropriate
// base URIs into the configuration, so a common pattern is:
//
//     dataSource := NewSDKDataSource(t, ...)
//     eventSink := NewSDKEventSink(t, ...)
//     client := NewSDKClient(t, dataSource, eventSink)
//
// Since the client will attempt to connect to its data source and possibly send events as soon as it
// starts up, the test fixtures must always be created first. You may reuse a previously created data
// source and event sink that was created in a parent test scope, if you do not need a new one for each
// client.
//
// The object's lifecycle is tied to the test scope that created it; it will be automatically closed
// when this test scope exits. It can be reused by subtests until then.
func NewSDKClient(t *ldtest.T, configurer SDKConfigurer, moreConfigurers ...SDKConfigurer) *SDKClient {
	client, err := TryNewSDKClient(t, configurer, moreConfigurers...)
	require.NoError(t, err)
	return client
}

func TryNewSDKClient(t *ldtest.T, configurer SDKConfigurer, moreConfigurers ...SDKConfigurer) (*SDKClient, error) {
	config := servicedef.SDKConfigParams{}
	configurer.ApplyConfiguration(&config)
	for _, c := range moreConfigurers {
		c.ApplyConfiguration(&config)
	}
	if config.Credential == "" {
		config.Credential = defaultSDKKey
	}
	if err := validateSDKConfig(config); err != nil {
		return nil, err
	}

	params := servicedef.CreateInstanceParams{
		Configuration: config,
		Tag:           t.ID().String(),
	}

	sdkClient, err := requireContext(t).harness.NewTestServiceEntity(params, "SDK client", t.DebugLogger())
	if err != nil {
		return nil, err
	}

	c := &SDKClient{
		sdkClientEntity: sdkClient,
		sdkConfig:       config,
	}
	t.Defer(func() {
		_ = c.Close()
	})
	return c, nil
}

func validateSDKConfig(config servicedef.SDKConfigParams) error {
	if !config.Streaming.IsDefined() && !config.Polling.IsDefined() {
		return errors.New(
			"neither streaing nor polling was enabled-- did you forget to include the SDKDataSource as a parameter?")
	}
	if config.Streaming.IsDefined() && config.Streaming.Value().BaseURI == "" {
		return errors.New("streaming was enabled but base URI was not set")
	}
	if config.Polling.IsDefined() && config.Polling.Value().BaseURI == "" {
		return errors.New("polling was enabled but base URI was not set")
	}
	if config.Events.IsDefined() && config.Events.Value().BaseURI == "" {
		return errors.New("events were enabled but base URI was not set--" +
			" did you forget to include the SDKEventSink as a parameter?")
	}
	return nil
}

// Close tells the test service to shut down the client instance. Normally this happens automatically at
// the end of a test.
func (c *SDKClient) Close() error {
	return c.sdkClientEntity.Close()
}

// EvaluateFlag tells the SDK client to evaluate a feature flag. This corresponds to calling one of the SDK's
// Variation or VariationDetail methods, depending on the parameters.
//
// Any error from the test service causes the test to terminate immediately.
func (c *SDKClient) EvaluateFlag(t *ldtest.T, params servicedef.EvaluateFlagParams) servicedef.EvaluateFlagResponse {
	if params.ValueType == "" {
		params.ValueType = servicedef.ValueTypeAny // it'd be easy for a test to forget to set this
	}
	var resp servicedef.EvaluateFlagResponse
	require.NoError(t, c.sdkClientEntity.SendCommandWithParams(
		servicedef.CommandParams{
			Command:  servicedef.CommandEvaluateFlag,
			Evaluate: o.Some(params),
		},
		t.DebugLogger(),
		&resp,
	))
	return resp
}

// EvaluateAllFlags tells the SDK client to evaluate all feature flags. This corresponds to calling the SDK's
// AllFlags or AllFlagsState method.
//
// Any error from the test service causes the test to terminate immediately.
func (c *SDKClient) EvaluateAllFlags(
	t *ldtest.T,
	params servicedef.EvaluateAllFlagsParams,
) servicedef.EvaluateAllFlagsResponse {
	var resp servicedef.EvaluateAllFlagsResponse
	require.NoError(t, c.sdkClientEntity.SendCommandWithParams(
		servicedef.CommandParams{
			Command:     servicedef.CommandEvaluateAllFlags,
			EvaluateAll: o.Some(params),
		},
		t.DebugLogger(),
		&resp,
	))
	return resp
}

// SendIdentifyEvent tells the SDK client to send an identify event.
//
// Any error from the test service causes the test to terminate immediately.
func (c *SDKClient) SendIdentifyEvent(t *ldtest.T, user lduser.User) {
	require.NoError(t, c.sdkClientEntity.SendCommandWithParams(
		servicedef.CommandParams{
			Command:       servicedef.CommandIdentifyEvent,
			IdentifyEvent: o.Some(servicedef.IdentifyEventParams{User: user}),
		},
		t.DebugLogger(),
		nil,
	))
}

// SendCustomEvent tells the SDK client to send a custom event.
//
// Any error from the test service causes the test to terminate immediately.
func (c *SDKClient) SendCustomEvent(t *ldtest.T, params servicedef.CustomEventParams) {
	require.NoError(t, c.sdkClientEntity.SendCommandWithParams(
		servicedef.CommandParams{
			Command:     servicedef.CommandCustomEvent,
			CustomEvent: o.Some(params),
		},
		t.DebugLogger(),
		nil,
	))
}

// SendAliasEvent tells the SDK client to send an alias event.
//
// Any error from the test service causes the test to terminate immediately.
func (c *SDKClient) SendAliasEvent(t *ldtest.T, params servicedef.AliasEventParams) {
	require.NoError(t, c.sdkClientEntity.SendCommandWithParams(
		servicedef.CommandParams{
			Command:    servicedef.CommandAliasEvent,
			AliasEvent: o.Some(params),
		},
		t.DebugLogger(),
		nil,
	))
}

// FlushEvents tells the SDK client to initiate an event flush.
//
// Any error from the test service causes the test to terminate immediately.
func (c *SDKClient) FlushEvents(t *ldtest.T) {
	require.NoError(t, c.sdkClientEntity.SendCommand(servicedef.CommandFlushEvents, t.DebugLogger(), nil))
}

// GetBigSegmentStoreStatus queries the big segment store status from the SDK client. The test
// harness will only call this method if the test service has the "big-segments" capability.
func (c *SDKClient) GetBigSegmentStoreStatus(t *ldtest.T) servicedef.BigSegmentStoreStatusResponse {
	var resp servicedef.BigSegmentStoreStatusResponse
	require.NoError(t, c.sdkClientEntity.SendCommand(servicedef.CommandGetBigSegmentStoreStatus,
		t.DebugLogger(), &resp))
	return resp
}
