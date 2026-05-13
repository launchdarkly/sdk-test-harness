package sdktests

import (
	"net/http"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
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
		dataSystem := NewSDKDataSystem(t, nil, DataSystemOptionStreaming())
		configurers := c.baseSDKConfigurationPlus(dataSystem)
		if c.isClientSide {
			// client-side SDKs in streaming mode may *also* need a polling data source
			configurers = append(configurers,
				NewSDKDataSystem(t, nil, DataSystemOptionPolling()))
		}
		_ = NewSDKClient(t, configurers...)
		verifyRequestHeader(t, dataSystem.Synchronizers[0].Endpoint())
	})

	if t.Capabilities().HasAny(servicedef.CapabilityClientSide, servicedef.CapabilityServerSidePolling) {
		t.Run("poll requests", func(t *ldtest.T) {
			dataSystem := NewSDKDataSystem(t, nil, DataSystemOptionPolling())
			_ = NewSDKClient(t, c.baseSDKConfigurationPlus(dataSystem)...)
			verifyRequestHeader(t, dataSystem.Synchronizers[0].Endpoint())
		})
	}

	t.Run("event posts", func(t *ldtest.T) {
		dataSystem := NewSDKDataSystem(t, nil)
		events := NewSDKEventSink(t)
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(
			dataSystem,
			events)...)

		c.sendArbitraryEvent(t, client)
		client.FlushEvents(t)

		verifyRequestHeader(t, events.Endpoint())
	})

	// FDv2 introduces request shapes that are not exercised by the streaming
	// or polling synchronizer subtests above: an Initializer request that
	// precedes the synchronizer, a Secondary Synchronizer that is only
	// contacted after the Primary is permanently removed, and an FDv1 Fallback
	// Synchronizer reached via the server-directed FDv1 Fallback Directive.
	// These shapes are server-side only today; gate accordingly.
	if !c.isClientSide {
		t.Run("polling initializer requests", func(t *ldtest.T) {
			initializerData := mockld.NewServerSDKDataBuilder().Build()
			synchronizerData := mockld.NewServerSDKDataBuilder().
				IntentCode("none").IntentReason("up-to-date").Build()
			dataSystem := NewSDKDataSystem(t, synchronizerData,
				DataSystemOptionPollingInitializer(initializerData))
			_ = NewSDKClient(t, c.baseSDKConfigurationPlus(dataSystem)...)
			verifyRequestHeader(t, dataSystem.Initializers[0].Endpoint())
		})

		t.Run("secondary synchronizer requests after permanent fallback", func(t *ldtest.T) {
			// Primary returns 401, a non-recoverable status that permanently
			// removes it from the synchronizer chain and causes the SDK to
			// fall through to the Secondary immediately.
			primaryEndpoint := requireContext(t).harness.NewMockEndpoint(
				httphelpers.HandlerWithStatus(401), t.DebugLogger(),
				harness.MockEndpointDescription("unauthorized primary streaming service"))
			t.Defer(primaryEndpoint.Close)

			secondaryStream := mockld.NewStreamingService(
				mockld.NewServerSDKDataBuilder().Build(),
				requireContext(t).sdkKind, t.DebugLogger())
			secondaryEndpoint := requireContext(t).harness.NewMockEndpoint(
				secondaryStream, t.DebugLogger(),
				harness.MockEndpointDescription("secondary streaming service"))
			t.Defer(secondaryEndpoint.Close)

			_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
				WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
					BaseURI: primaryEndpoint.BaseURL(),
				}),
				WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
					BaseURI: secondaryEndpoint.BaseURL(),
				}))...)

			verifyRequestHeader(t, secondaryEndpoint)
		})
	}

	if t.Capabilities().Has(servicedef.CapabilityFDv1Fallback) {
		t.Run("FDv1 fallback directive requests", func(t *ldtest.T) {
			// FDv2 streaming responds with 403 + directive on every request
			// so the SDK transitions to the FDv1 Fallback Synchronizer. The
			// FDv1 endpoint serves an empty payload so initialization can
			// complete along the fallback path.
			streamEndpoint := requireContext(t).harness.NewMockEndpoint(
				httphelpers.HandlerWithResponse(
					403, http.Header{"X-LD-FD-Fallback": []string{"true"}}, nil),
				t.DebugLogger(),
				harness.MockEndpointDescription("FDv2 streaming service (403 + directive)"))
			t.Defer(streamEndpoint.Close)

			fdv1Endpoint := requireContext(t).harness.NewMockEndpoint(
				httphelpers.HandlerWithResponse(
					200,
					http.Header{"Content-Type": []string{"application/json"}},
					[]byte(`{"flags":{},"segments":{}}`)),
				t.DebugLogger(),
				harness.MockEndpointDescription("FDv1 polling service"))
			t.Defer(fdv1Endpoint.Close)

			_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
				WithConfig(servicedef.SDKConfigParams{
					StartWaitTimeMS: o.Some(ldtime.UnixMillisecondTime(5000)),
				}),
				WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
					BaseURI: streamEndpoint.BaseURL(),
				}),
				WithFDv1Fallback(servicedef.SDKConfigPollingParams{
					BaseURI: fdv1Endpoint.BaseURL(),
				}))...)

			verifyRequestHeader(t, fdv1Endpoint)
		})
	}
}

func (c CommonInstanceIDTests) RunPHP(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityInstanceID)

	verifyRequestHeader := func(t *ldtest.T, endpoint *harness.MockEndpoint) {
		request := endpoint.RequireConnection(t, time.Second)
		assert.NotEmpty(t, request.Headers.Get("X-LaunchDarkly-Instance-Id"))
	}

	t.Run("poll requests", func(t *ldtest.T) {
		dataSystem := NewSDKDataSystem(t, nil)
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSystem)...)
		client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
			FlagKey:      "flag-key",
			Context:      o.Some(ldcontext.New("key")),
			ValueType:    servicedef.ValueTypeBool,
			DefaultValue: ldvalue.Bool(false),
			Detail:       false,
		})

		verifyRequestHeader(t, dataSystem.Synchronizers[0].Endpoint())
	})

	t.Run("event posts", func(t *ldtest.T) {
		dataSystem := NewSDKDataSystem(t, nil)
		events := NewSDKEventSink(t)
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(
			dataSystem,
			events)...)

		c.sendArbitraryEvent(t, client)
		client.FlushEvents(t)

		verifyRequestHeader(t, events.Endpoint())
	})
}
