package sdktests

import (
	"strings"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/require"
)

func doClientSideStreamTests(t *ldtest.T) {
	t.Run("requests", doClientSideStreamRequestTest)
	t.Run("updates", doClientSideStreamUpdateTests)
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
		case mockld.RokuSDK:
			panic("invalid SDK kind")
		case mockld.MobileSDK:
			mobileGetPathPrefix := strings.TrimSuffix(mockld.StreamingPathMobileGet, mockld.StreamingPathContextBase64Param)
			return h.IfElse(method == flagRequestREPORT,
				m.Equal("/meval"),
				m.StringHasPrefix(mobileGetPathPrefix))
			// details of base64-encoded context data are tested separately

		case mockld.JSClientSDK:
			jsGetPathPrefix := strings.TrimSuffix(
				strings.ReplaceAll(mockld.StreamingPathJSClientGet, mockld.StreamingPathEnvIDParam, envIDOrMobileKey),
				mockld.StreamingPathContextBase64Param, // details of base64-encoded context data are tested separately
			)
			jsReportPath := strings.ReplaceAll(mockld.StreamingPathJSClientReport,
				mockld.StreamingPathEnvIDParam, envIDOrMobileKey)
			return h.IfElse(method == flagRequestREPORT,
				m.Equal(jsReportPath),
				m.StringHasPrefix(jsGetPathPrefix))

		default:
			panic("invalid SDK kind")
		}
	}
	streamTests.RequestURLPath(t, requestPathMatcher)

	getPath := h.IfElse(sdkKind == mockld.MobileSDK || sdkKind == mockld.RokuSDK,
		mockld.StreamingPathMobileGet,
		strings.ReplaceAll(mockld.StreamingPathJSClientGet, mockld.PollingPathEnvIDParam, envIDOrMobileKey))
	streamTests.RequestContextProperties(t, getPath)
}

func doClientSideStreamUpdateTests(t *ldtest.T) {
	NewCommonStreamingTests(t, "doClientSideStreamUpdateTests").Updates(t)
}

func doClientSideStreamConnectionLifecycleTests(t *ldtest.T) {
	// This test verifies that when the SDK client is closed, it actively closes its streaming
	// connection rather than leaving the underlying TCP socket lingering. Go's HTTP server cancels
	// the incoming request's Context when the client closes the underlying TCP connection, so we
	// detect closure by waiting for that Context to be cancelled.
	//
	// It explicitly configures a streaming data source, since JS-based client-side SDKs default to
	// polling. It is intentionally not gated behind any FDv2 capability.
	t.Run("SDK closes streaming connection when client is closed", func(t *ldtest.T) {
		dataSource := NewSDKDataSource(t, nil, DataSourceOptionStreaming())

		client := NewSDKClient(t,
			WithClientSideInitialContext(ldcontext.New("user-key")),
			dataSource)

		streamRequest := dataSource.Endpoint().RequireConnection(t, time.Second*5)

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
