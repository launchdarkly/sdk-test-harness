package sdktests

import (
	"net/http"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
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

	t.Run("stream requests", func(t *ldtest.T) {
		dataSystem := NewSDKDataSystem(t, nil, DataSystemOptionStreaming())
		configurers := c.baseSDKConfigurationPlus(dataSystem)
		if c.isClientSide {
			// client-side SDKs in streaming mode may *also* need a polling data source
			configurers = append(configurers,
				NewSDKDataSystem(t, nil, DataSystemOptionPolling()))
		}
		_ = NewSDKClient(t, configurers...)
		check := newInstanceIDChecker(t)
		check(dataSystem.Synchronizers[0].Endpoint())
	})

	if t.Capabilities().HasAny(servicedef.CapabilityClientSide, servicedef.CapabilityServerSidePolling) {
		t.Run("poll requests", func(t *ldtest.T) {
			dataSystem := NewSDKDataSystem(t, nil, DataSystemOptionPolling())
			_ = NewSDKClient(t, c.baseSDKConfigurationPlus(dataSystem)...)
			check := newInstanceIDChecker(t)
			check(dataSystem.Synchronizers[0].Endpoint())
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

		// The SDK contacts the data source during init and the events endpoint
		// on flush; both must carry the same instance-id since they originate
		// from the same client.
		check := newInstanceIDChecker(t)
		check(dataSystem.Synchronizers[0].Endpoint())
		check(events.Endpoint())
	})

	// instance-id identifies an SDK client instance; two distinct clients
	// living in the same process must never share a value, or telemetry can't
	// disambiguate them. Stand up two independent clients back to back and
	// assert their instance-ids differ. Gated on !CapabilitySingleton since
	// the test requires creating a second client while the first still exists.
	if !t.Capabilities().Has(servicedef.CapabilitySingleton) {
		t.Run("instance id differs between client instances", func(t *ldtest.T) {
			captureInstanceID := func() string {
				dataSystem := NewSDKDataSystem(t, nil, DataSystemOptionStreaming())
				configurers := c.baseSDKConfigurationPlus(dataSystem)
				if c.isClientSide {
					// client-side SDKs in streaming mode may *also* need a
					// polling data source
					configurers = append(configurers,
						NewSDKDataSystem(t, nil, DataSystemOptionPolling()))
				}
				_ = NewSDKClient(t, configurers...)
				request := dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)
				v := request.Headers.Get("X-LaunchDarkly-Instance-Id")
				assert.NotEmpty(t, v, "X-LaunchDarkly-Instance-Id missing from request")
				return v
			}

			first := captureInstanceID()
			second := captureInstanceID()

			assert.NotEqual(t, first, second,
				"two distinct SDK client instances must have distinct "+
					"X-LaunchDarkly-Instance-Id values")
		})
	}

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
			check := newInstanceIDChecker(t)
			check(dataSystem.Initializers[0].Endpoint())
			check(dataSystem.Synchronizers[0].Endpoint())
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

			check := newInstanceIDChecker(t)
			check(primaryEndpoint)
			check(secondaryEndpoint)
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
				WithWaitToStart(5*time.Second, false),
				WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
					BaseURI: streamEndpoint.BaseURL(),
				}),
				WithFDv1Fallback(servicedef.SDKConfigPollingParams{
					BaseURI: fdv1Endpoint.BaseURL(),
				}))...)

			check := newInstanceIDChecker(t)
			check(streamEndpoint)
			check(fdv1Endpoint)
		})
	}
}

// newInstanceIDChecker returns a function that asserts every observed request
// carries a non-empty X-LaunchDarkly-Instance-Id header AND that the value is
// identical across every endpoint observed by the returned checker. The
// instance-id identifies the SDK client instance, so it must be stable for the
// client's lifetime no matter which request shape carries it. Each subtest
// creates its own SDK client and so should create its own checker -- the
// latched value is per-client.
func newInstanceIDChecker(t *ldtest.T) func(*harness.MockEndpoint) {
	var observed string
	return func(endpoint *harness.MockEndpoint) {
		t.Helper()
		request := endpoint.RequireConnection(t, time.Second)
		v := request.Headers.Get("X-LaunchDarkly-Instance-Id")
		if !assert.NotEmpty(t, v, "X-LaunchDarkly-Instance-Id missing from request") {
			return
		}
		if observed == "" {
			observed = v
			return
		}
		assert.Equal(t, observed, v,
			"X-LaunchDarkly-Instance-Id differs across requests from the same SDK client")
	}
}

func (c CommonInstanceIDTests) RunPHP(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityInstanceID)

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

		check := newInstanceIDChecker(t)
		check(dataSystem.Synchronizers[0].Endpoint())
	})

	t.Run("event posts", func(t *ldtest.T) {
		dataSystem := NewSDKDataSystem(t, nil)
		events := NewSDKEventSink(t)
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(
			dataSystem,
			events)...)

		c.sendArbitraryEvent(t, client)
		client.FlushEvents(t)

		check := newInstanceIDChecker(t)
		check(events.Endpoint())
	})
}
