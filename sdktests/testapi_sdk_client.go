package sdktests

import (
	"errors"
	"sync/atomic"

	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"

	"github.com/stretchr/testify/require"
)

var currentlyExistingClientInstances int32                                         //nolint:gochecknoglobals
var arbitraryInitialContexts = data.NewContextFactory("arbitrary-initial-context") //nolint:gochecknoglobals

// SDKConfigurer is an interface for objects that can modify the configuration for StartSDKClient.
// It is implemented by types such as SDKDataSource.
type SDKConfigurer helpers.ConfigOption[servicedef.SDKConfigParams]

// WithConfig is used with StartSDKClient to specify a non-default SDK configuration. Use this
// before any other SDKConfigurers or it will overwrite their effects.
func WithConfig(config servicedef.SDKConfigParams) SDKConfigurer {
	return helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(configOut *servicedef.SDKConfigParams) error {
		*configOut = config
		return nil
	})
}

// WithCredential is used with StartSDKClient to set only the credential (SDK key, mobile key, or
// environment ID).
func WithCredential(credential string) SDKConfigurer {
	return helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(configOut *servicedef.SDKConfigParams) error {
		configOut.Credential = credential
		return nil
	})
}

// WithClientSideConfig is used with StartSDKClient to specify a non-default client-side SDK
// configuration.
func WithClientSideConfig(clientSideConfig servicedef.SDKConfigClientSideParams) SDKConfigurer {
	return helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(configOut *servicedef.SDKConfigParams) error {
		configOut.ClientSide = o.Some(clientSideConfig)
		return nil
	})
}

// WithEventsConfig is used with StartSDKClient to specify a non-default events configuration.
func WithEventsConfig(eventsConfig servicedef.SDKConfigEventParams) SDKConfigurer {
	return helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(configOut *servicedef.SDKConfigParams) error {
		configOut.Events = o.Some(eventsConfig)
		return nil
	})
}

// WithPollingConfig is used with StartSDKClient to specify a non-default polling configuration.
func WithPollingConfig(pollingConfig servicedef.SDKConfigPollingParams) SDKConfigurer {
	return helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(configOut *servicedef.SDKConfigParams) error {
		configOut.Polling = o.Some(pollingConfig)
		return nil
	})
}

// WithServiceEndpointsConfig is used with StartSDKClient to specify non-default service endpoints.
// This will only work if the test service has the "service-endpoints" capability.
func WithServiceEndpointsConfig(endpointsConfig servicedef.SDKConfigServiceEndpointsParams) SDKConfigurer {
	return helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(configOut *servicedef.SDKConfigParams) error {
		configOut.ServiceEndpoints = o.Some(endpointsConfig)
		return nil
	})
}

// WithStreamingConfig is used with StartSDKClient to specify a non-default streaming configuration.
func WithStreamingConfig(streamingConfig servicedef.SDKConfigStreamingParams) SDKConfigurer {
	return helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(configOut *servicedef.SDKConfigParams) error {
		configOut.Streaming = o.Some(streamingConfig)
		return nil
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
func NewSDKClient(t *ldtest.T, configurers ...SDKConfigurer) *SDKClient {
	count := atomic.AddInt32(&currentlyExistingClientInstances, 1)
	if count == 2 && t.Capabilities().Has(servicedef.CapabilitySingleton) {
		atomic.AddInt32(&currentlyExistingClientInstances, -1)
		t.Errorf("Test tried to create an SDK client instance when one already existed, and this SDK is a singleton")
		t.FailNow()
	}
	client, err := TryNewSDKClient(t, configurers...)
	if err != nil {
		atomic.AddInt32(&currentlyExistingClientInstances, -1)
	}
	require.NoError(t, err)
	return client
}

func TryNewSDKClient(t *ldtest.T, configurers ...SDKConfigurer) (*SDKClient, error) {
	if len(configurers) == 0 {
		return nil, errors.New("tried to create an SDK client without any custom configuration")
	}

	config := servicedef.SDKConfigParams{}
	if err := helpers.ApplyOptions(&config, configurers...); err != nil {
		return nil, err
	}
	if config.Credential == "" {
		config.Credential = defaultSDKKey
	}
	if t.Capabilities().Has(servicedef.CapabilityClientSide) {
		// Ensure that we always provide an initial context for every client-side SDK test, if the test logic
		// didn't explicitly set one. It's preferable for this to have a unique key, so that if the SDK has any
		// global state that is cached by key, tests won't interfere with each other.
		if !config.ClientSide.Value().InitialContext.IsDefined() {
			cs := config.ClientSide.Value()
			cs.InitialContext = arbitraryInitialContexts.NextUniqueContext()
			config.ClientSide = o.Some(cs)
		}
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
	if !config.Streaming.IsDefined() && !config.Polling.IsDefined() && config.ServiceEndpoints.Value().Streaming == "" {
		// Note that the default is streaming, so we don't necessarily need to set config.Streaming if there are
		// no other customized options and if we used serviceEndpoints.streaming to set the stream URI
		return errors.New(
			"neither streaming nor polling was enabled-- did you forget to include the SDKDataSource as a parameter?")
	}
	if config.Streaming.IsDefined() && config.Streaming.Value().BaseURI == "" &&
		(!config.ServiceEndpoints.IsDefined() || config.ServiceEndpoints.Value().Streaming == "") {
		return errors.New("streaming was enabled but base URI was not set")
	}
	if config.Polling.IsDefined() && config.Polling.Value().BaseURI == "" &&
		(!config.ServiceEndpoints.IsDefined() || config.ServiceEndpoints.Value().Polling == "") {
		return errors.New("polling was enabled but base URI was not set")
	}
	if config.Events.IsDefined() && config.Events.Value().BaseURI == "" &&
		(!config.ServiceEndpoints.IsDefined() || config.ServiceEndpoints.Value().Events == "") {
		return errors.New("events were enabled but base URI was not set--" +
			" did you forget to include the SDKEventSink as a parameter?")
	}
	return nil
}

// Close tells the test service to shut down the client instance. Normally this happens automatically at
// the end of a test.
func (c *SDKClient) Close() error {
	atomic.AddInt32(&currentlyExistingClientInstances, -1)
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
func (c *SDKClient) SendIdentifyEvent(t *ldtest.T, context ldcontext.Context) {
	require.NoError(t, c.sdkClientEntity.SendCommandWithParams(
		servicedef.CommandParams{
			Command:       servicedef.CommandIdentifyEvent,
			IdentifyEvent: o.Some(servicedef.IdentifyEventParams{Context: context}),
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

// ContextBuild tells the test service to use the SDK's context builder to build a context and return it as JSON.
func (c *SDKClient) ContextBuild(t *ldtest.T, params servicedef.ContextBuildParams) servicedef.ContextBuildResponse {
	var resp servicedef.ContextBuildResponse
	require.NoError(t, c.sdkClientEntity.SendCommandWithParams(
		servicedef.CommandParams{
			Command:      servicedef.CommandContextBuild,
			ContextBuild: o.Some(params),
		},
		t.DebugLogger(),
		&resp,
	))
	return resp
}

// ContextConvert tells the test service to use the SDK's JSON converters to unmarshal and remarshal a context.
func (c *SDKClient) ContextConvert(
	t *ldtest.T,
	params servicedef.ContextConvertParams,
) servicedef.ContextBuildResponse {
	var resp servicedef.ContextBuildResponse
	require.NoError(t, c.sdkClientEntity.SendCommandWithParams(
		servicedef.CommandParams{
			Command:        servicedef.CommandContextConvert,
			ContextConvert: o.Some(params),
		},
		t.DebugLogger(),
		&resp,
	))
	return resp
}

// GetSecureModeHash tells the SDK client to calculate a secure mode hash for a context. The test
// harness will only call this method if the test service has the "secure-mode-hash" capability.
func (c *SDKClient) GetSecureModeHash(t *ldtest.T, context ldcontext.Context) string {
	var resp servicedef.SecureModeHashResponse
	require.NoError(t, c.sdkClientEntity.SendCommandWithParams(
		servicedef.CommandParams{
			Command:        servicedef.CommandSecureModeHash,
			SecureModeHash: o.Some(servicedef.SecureModeHashParams{Context: context}),
		},
		t.DebugLogger(),
		&resp))
	return resp.Result
}
