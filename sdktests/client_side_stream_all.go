package sdktests

import (
	"strings"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/require"
)

func doClientSideStreamTests(t *ldtest.T) {
	t.Run("requests", doClientSideStreamRequestTest)
	t.Run("fdv2", doClientSideFDv2StreamTests)
	t.Run("connection lifecycle", doClientSideStreamConnectionLifecycleTests)
}

func doClientSideStreamRequestTest(t *ldtest.T) {
	sdkKind := requireContext(t).sdkKind
	envIDOrMobileKey := "my-credential"

	streamTests := NewCommonStreamingTests(t, "doClientSideStreamRequestTest",
		WithCredential(envIDOrMobileKey))

	if t.Capabilities().Has(servicedef.CapabilityHTTPProxy) {
		streamTests.RequestViaHTTPProxy(t)
	}

	streamTests.RequestMethodAndHeaders(t, envIDOrMobileKey)

	requestPathMatcher := func(method flagRequestMethod) m.Matcher {
		switch sdkKind {
		case mockld.MobileSDK, mockld.JSClientSDK:
			getPathPrefix := strings.TrimSuffix(
				mockld.StreamingPathFDv2ClientGet,
				mockld.StreamingPathContextBase64Param, // details of base64-encoded context data are tested separately
			)
			return h.IfElse(method == flagRequestREPORT,
				m.Equal(mockld.StreamingPathFDv2ClientPost),
				m.StringHasPrefix(getPathPrefix))
		case mockld.RokuSDK:
			panic("invalid SDK kind")
		default:
			panic("invalid SDK kind")
		}
	}
	streamTests.RequestURLPath(t, requestPathMatcher)

	getPath := h.IfElse(sdkKind == mockld.RokuSDK,
		mockld.StreamingPathMobileGet,
		mockld.StreamingPathFDv2ClientGet)
	streamTests.RequestContextProperties(t, getPath)
}

func doClientSideFDv2StreamTests(t *ldtest.T) {
	NewCommonStreamingTests(t, "doClientSideFDv2StreamTests").FDv2(t)
}

func doClientSideStreamConnectionLifecycleTests(t *ldtest.T) {
	// This test verifies that when the SDK client is closed, it actively closes its streaming
	// connection rather than leaving the underlying TCP socket lingering. Go's HTTP server cancels
	// the incoming request's Context when the client closes the underlying TCP connection, so we
	// detect closure by waiting for that Context to be cancelled.
	//
	// setupDataSystems configures the streaming synchronizer plus, for client-side SDKs, a polling
	// initializer for the initial payload (those SDKs poll first, then stream for updates).
	t.Run("SDK closes streaming connection when client is closed", func(t *ldtest.T) {
		streamTests := NewCommonStreamingTests(t, "doClientSideStreamConnectionLifecycleTests")
		dataSystem, configurers := streamTests.setupDataSystems(t, nil)

		client := streamTests.newFDv2SDKClient(t, configurers...)

		// Skip Roku's short-lived POST /handshake to the streaming endpoint (its Context is already
		// cancelled once the handler returns) so we assert against the long-lived SSE connection.
		endpoint := dataSystem.Synchronizers[0].Endpoint()
		deadline := time.Now().Add(time.Second * 5)
		streamRequest := endpoint.RequireConnection(t, time.Until(deadline))
		for streamRequest.URL.Path == mockld.StreamingPathRokuHandshake {
			streamRequest = endpoint.RequireConnection(t, time.Until(deadline))
		}

		// Closing the client should force the SDK to close its streaming connection. This is
		// idempotent with the automatic close that happens at end-of-test.
		require.NoError(t, client.Close())

		h.RequireEventually(
			t,
			func() bool {
				select {
				case <-streamRequest.Context.Done():
					return true
				default:
					return false
				}
			},
			time.Second*3,
			time.Millisecond*20,
			"SDK did not close the streaming connection after the client was closed",
		)
	})
}
