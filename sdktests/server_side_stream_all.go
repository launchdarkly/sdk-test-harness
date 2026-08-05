package sdktests

import (
	"time"

	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/require"
)

func doServerSideStreamTests(t *ldtest.T) {
	t.Run("requests", doServerSideStreamRequestTests)
	t.Run("retry behavior", doServerSideStreamRetryTests)
	t.Run("validation", doServerSideStreamValidationTests)

	t.Run("fdv2", doServerSideFDv2StreamTests)
	t.Run("connection lifecycle", doServerSideStreamConnectionLifecycleTests)
}

func doServerSideStreamRequestTests(t *ldtest.T) {
	sdkKey := "my-sdk-key"

	streamTests := NewCommonStreamingTests(t, "doServerSideStreamRequestTests",
		WithConfig(servicedef.SDKConfigParams{
			Credential: sdkKey,
		}))

	if t.Capabilities().Has(servicedef.CapabilityHTTPProxy) {
		streamTests.RequestViaHTTPProxy(t)
	}

	streamTests.RequestMethodAndHeaders(t, sdkKey)

	streamTests.RequestURLPath(t, func(flagRequestMethod) m.Matcher {
		return m.Equal(mockld.StreamingPathServerSide)
	})
}

func doServerSideFDv2StreamTests(t *ldtest.T) {
	NewCommonStreamingTests(t, "doServerSideFDv2StreamTests").FDv2(t)
}

func doServerSideStreamConnectionLifecycleTests(t *ldtest.T) {
	// This test verifies that when the SDK client is closed, it actively closes its streaming
	// connection rather than leaving the underlying TCP socket lingering. Go's HTTP server cancels
	// the incoming request's Context when the client closes the underlying TCP connection, so we
	// detect closure by waiting for that Context to be cancelled.
	t.Run("SDK closes streaming connection when client is closed", func(t *ldtest.T) {
		streamTests := NewCommonStreamingTests(t, "doServerSideStreamConnectionLifecycleTests")
		dataSystem, configurers := streamTests.setupDataSystems(t, nil)

		client := streamTests.newFDv2SDKClient(t, configurers...)

		streamRequest := dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second*5)

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
