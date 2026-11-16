package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/require"
)

// The poll interval for these tests. Server-side SDKs do not allow a shorter one.
const serverSidePollInterval = time.Second * 30

// doServerSideRepeatedPollTests verifies how the FDv1 polling data source behaves after its first
// poll. Each test waits for more than one poll interval, so these tests are long-running.
func doServerSideRepeatedPollTests(t *ldtest.T) {
	t.LongRunning()

	t.Run("no further polls after the client is closed", func(t *ldtest.T) {
		flagKey := "flag"
		value, defaultValue := ldvalue.String("value"), ldvalue.String("default")
		flag := ldbuilders.NewFlagBuilder(flagKey).Version(1).On(false).OffVariation(0).Variations(value).Build()
		source := NewSDKDataSourceWithoutEndpoint(t, mockld.NewServerSDKDataBuilder().Flag(flag).Build(),
			DataSourceOptionPolling())
		endpoint := requireContext(t).harness.NewMockEndpoint(source.PollingService(), t.DebugLogger(),
			harness.MockEndpointDescription("polling service"))
		t.Defer(endpoint.Close)

		client := NewSDKClient(t, WithPollingConfig(servicedef.SDKConfigPollingParams{
			BaseURI:        endpoint.BaseURL(),
			PollIntervalMS: o.Some(ldtime.UnixMillisecondTime(serverSidePollInterval / time.Millisecond)),
		}))
		_ = endpoint.RequireConnection(t, time.Second*10)
		m.In(t).Assert(basicEvaluateFlag(t, client, flagKey, ldcontext.New("user-key"), defaultValue), m.Equal(value))

		// Close the client, then wait for longer than the poll interval.
		require.NoError(t, client.Close())
		// The SDK must not poll again.
		endpoint.RequireNoMoreConnections(t, serverSidePollInterval*2)
	})
}
