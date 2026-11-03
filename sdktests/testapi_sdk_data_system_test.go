package sdktests

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"

	"github.com/stretchr/testify/require"
)

// newFakeTestServiceForDataSystemTest starts an httptest server that satisfies the minimal
// contract queryTestServiceInfo (framework/harness/test_service.go) requires of a real SDK
// test service: a GET to the root URL must return a 2xx status with a JSON body containing
// at least "name" and "capabilities". No capabilities are needed here, since
// SDKTestContext.sdkKind is set directly rather than derived from them.
func newFakeTestServiceForDataSystemTest(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"name":"fake-test-service","capabilities":[]}`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// freeTCPPortForDataSystemTest returns a currently-unused TCP port on localhost.
//
// There's an inherent (very small) race between closing this probe listener and
// harness.NewTestHarness binding its own listener to the same port. This is accepted:
// harness.NewTestHarness requires a concrete port number up front (it has no
// ephemeral-port support, and its mock-endpoint URLs are built from this port number).
func freeTCPPortForDataSystemTest(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// newTestHarnessForDataSystemTest builds a real *harness.TestHarness backed by a fake
// httptest test-service, so SDKDataSystem.Configure() can be exercised end-to-end
// (including real harness.MockEndpoint creation) without a live SDK test-service process.
//
// harness.NewTestHarness's constructor eagerly queries the test service and binds a real
// TCP listener (unlike harness.MockEndpoint.BaseURL(), which is pure string construction
// from host/port/id), so a real, working *harness.TestHarness must exist up front.
func newTestHarnessForDataSystemTest(t *testing.T) *harness.TestHarness {
	fakeService := newFakeTestServiceForDataSystemTest(t)
	port := freeTCPPortForDataSystemTest(t)
	h, err := harness.NewTestHarness(
		fakeService.URL,
		"127.0.0.1",
		port,
		false,
		5*time.Second,
		nil,
		io.Discard,
	)
	require.NoError(t, err)
	return h
}

// TestClientSideStreamModeSecondaryPollingDataSystemDoesNotCollide is a regression test
// guarding against a connection-mode name collision. CommonWrapperTests's "stream
// requests/wrapper name and version" subtest creates a primary client-side SDKDataSystem
// in streaming mode plus a secondary "poll before stream" data system
// (clientSideSecondaryPollingDataSystem, in common_tests_wrapper.go). Both used to be
// keyed under the same connection-mode name ("streaming"). SDKDataSystem.Configure()
// overwrites rather than merges same-named entries in the shared
// servicedef.SDKConfigParams, so the secondary's Configure() call was silently
// clobbering the primary's endpoint URIs.
func TestClientSideStreamModeSecondaryPollingDataSystemDoesNotCollide(t *testing.T) {
	h := newTestHarnessForDataSystemTest(t)

	results := ldtest.Run(ldtest.TestConfiguration{
		Context: SDKTestContext{harness: h, sdkKind: mockld.MobileSDK, protocol: mockld.FDv2},
	}, func(scope *ldtest.T) {
		primary := NewSDKDataSystem(scope, nil, DataSystemOptionStreaming())
		secondaryConfigurer := clientSideSecondaryPollingDataSystem(scope)

		var config servicedef.SDKConfigParams
		require.NoError(t, primary.Configure(&config))
		require.NoError(t, secondaryConfigurer.Configure(&config))

		dataSystem := config.DataSystem.Value()
		connMode := dataSystem.ConnectionModeConfig.Value()
		modes := connMode.CustomConnectionModes.Value()

		streamingMode, ok := modes["streaming"]
		require.True(t, ok, `expected a "streaming" connection mode to be present`)
		require.Len(t, streamingMode.Synchronizers, 1)
		require.True(t, streamingMode.Synchronizers[0].Streaming.IsDefined(),
			`expected the "streaming" mode's synchronizer to still be a streaming synchronizer`)

		expectedBaseURI := primary.Synchronizers[0].Endpoint().BaseURL()
		actualBaseURI := streamingMode.Synchronizers[0].Streaming.Value().BaseURI
		require.Equal(t, expectedBaseURI, actualBaseURI,
			`the primary data system's own "streaming" mode endpoint was overwritten by the secondary data system's Configure() call`)

		require.Equal(t, "streaming", connMode.InitialConnectionMode.Value(),
			`expected the SDK to still start in the primary's "streaming" mode`)

		pollingMode, ok := modes["polling"]
		require.True(t, ok, `expected a distinctly-named "polling" connection mode for the secondary data system`)
		require.Len(t, pollingMode.Synchronizers, 1)
		require.True(t, pollingMode.Synchronizers[0].Polling.IsDefined(),
			`expected the "polling" mode's synchronizer to be a polling synchronizer`)
	})
	require.True(t, results.OK(), "test scope failed: %+v", results.Failures)
}

// runWithFDv1Context runs action in a test scope for the FDv1 branch, and returns the results.
func runWithFDv1Context(t *testing.T, sdkKind mockld.SDKKind, action func(*ldtest.T)) ldtest.Results {
	h := newTestHarnessForDataSystemTest(t)
	return ldtest.Run(ldtest.TestConfiguration{
		Context: SDKTestContext{harness: h, sdkKind: sdkKind, protocol: mockld.FDv1},
	}, action)
}

func TestFDv1DataSystemConfiguresOneDataService(t *testing.T) {
	for _, p := range []struct {
		name            string
		sdkKind         mockld.SDKKind
		options         []SDKDataSystemOption
		expectStreaming bool
		expectPolling   bool
	}{
		{"server-side default", mockld.ServerSideSDK, nil, true, false},
		{"server-side polling", mockld.ServerSideSDK, []SDKDataSystemOption{DataSystemOptionPolling()}, false, true},
		{"PHP default", mockld.PHPSDK, nil, false, true},
		{"mobile default", mockld.MobileSDK, nil, true, false},
		{"JS client default", mockld.JSClientSDK, nil, false, true},
		{"client-side polling connection mode", mockld.MobileSDK, []SDKDataSystemOption{
			DataSystemOptionConnectionMode("polling", DataSystemOptionPolling()),
			DataSystemOptionConnectionMode("streaming", DataSystemOptionStreaming()),
			DataSystemOptionInitialConnectionMode("polling"),
		}, false, true},
	} {
		t.Run(p.name, func(t *testing.T) {
			results := runWithFDv1Context(t, p.sdkKind, func(scope *ldtest.T) {
				dataSystem := NewSDKDataSystem(scope, nil, p.options...)
				var config servicedef.SDKConfigParams
				require.NoError(t, dataSystem.Configure(&config))

				require.False(t, config.DataSystem.IsDefined(), "FDv1 configuration must not have a data system")
				require.Equal(t, p.expectStreaming, config.Streaming.IsDefined(), "streaming configuration")
				require.Equal(t, p.expectPolling, config.Polling.IsDefined(), "polling configuration")
				require.Len(t, dataSystem.Synchronizers, 1)
				baseURI := dataSystem.Synchronizers[0].Endpoint().BaseURL()
				if p.expectStreaming {
					require.Equal(t, baseURI, config.Streaming.Value().BaseURI)
				} else {
					require.Equal(t, baseURI, config.Polling.Value().BaseURI)
				}
			})
			require.True(t, results.OK(), "test scope failed: %+v", results.Failures)
		})
	}
}

func TestFDv1DataSystemConfiguresClientSideStreamingWithInitialPoll(t *testing.T) {
	results := runWithFDv1Context(t, mockld.JSClientSDK, func(scope *ldtest.T) {
		dataSystem := NewSDKDataSystemCustom(scope, nil,
			DataSystemOptionPollingInitializer(nil), DataSystemOptionStreaming())
		dataSystem.CreateEndpoints()
		config := servicedef.SDKConfigParams{
			Streaming: o.Some(servicedef.SDKConfigStreamingParams{InitialRetryDelayMS: o.Some(ldtime.UnixMillisecondTime(5))}),
		}
		require.NoError(t, dataSystem.Configure(&config))

		require.Equal(t, dataSystem.Synchronizers[0].Endpoint().BaseURL(), config.Streaming.Value().BaseURI)
		require.Equal(t, ldtime.UnixMillisecondTime(5), config.Streaming.Value().InitialRetryDelayMS.Value(),
			"Configure must keep the streaming properties that a previous configurer set")
		require.Equal(t, dataSystem.Initializers[0].Endpoint().BaseURL(), config.Polling.Value().BaseURI)
	})
	require.True(t, results.OK(), "test scope failed: %+v", results.Failures)
}

func TestFDv1DataSystemConfiguresPollInterval(t *testing.T) {
	results := runWithFDv1Context(t, mockld.ServerSideSDK, func(scope *ldtest.T) {
		dataSystem := NewSDKDataSystem(scope, nil, DataSystemOptionPolling(),
			DataSystemOptionPollInterval(time.Minute))
		var config servicedef.SDKConfigParams
		require.NoError(t, dataSystem.Configure(&config))

		require.Equal(t, ldtime.UnixMillisecondTime(60000), config.Polling.Value().PollIntervalMS.Value())
	})
	require.True(t, results.OK(), "test scope failed: %+v", results.Failures)
}

func TestFDv1DataSystemRejectsServerSideInitializer(t *testing.T) {
	results := runWithFDv1Context(t, mockld.ServerSideSDK, func(scope *ldtest.T) {
		_ = NewSDKDataSystemCustom(scope, nil, DataSystemOptionPollingInitializer(nil), DataSystemOptionStreaming())
	})
	require.Len(t, results.Failures, 1, "an FDv1 server-side data system with an initializer must fail the test")
	require.Len(t, results.Failures[0].Errors, 1)
	require.Contains(t, results.Failures[0].Errors[0].Error(), "can have a polling initializer only if")
}
