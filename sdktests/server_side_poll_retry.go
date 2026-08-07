package sdktests

import (
	"fmt"
	"net/http"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
)

// doServerSidePollRetryTests exercises the RETRY specification's non-permanent-stop guarantee
// for the FDv1 server-side polling data source. Server-side polling enforces a PollInterval
// minimum of 30 seconds, so tests here inherently take tens of seconds per iteration.
//
// This first pass covers the core guarantee (no permanent stop after an unexpected HTTP error);
// additional polling scenarios (extended-regime shape, reset after 2 consecutive successful
// polls) are deferred to follow-up work.
func doServerSidePollRetryTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityServerSidePolling)
	t.RequireCapability(servicedef.CapabilityRetryConformanceFDv1Polling)

	unexpectedErrors := []int{401, 403, 405}

	expectedValue := ldvalue.Int(1)
	flagKey := "flag"
	flagV1, _ := makeFlagVersionsWithValues(flagKey, 1, 2, expectedValue, ldvalue.Int(2))
	dataV1 := mockld.NewServerSDKDataBuilder().Flag(flagV1).Build()
	context := ldcontext.New("user-key")

	// pollingRecoveryTimeout is a generous ceiling for observing the second poll request after
	// an unexpected error. Effective delay is max(ExtendedInitialDelayMS, PollInterval); with
	// PollInterval floored at the server-side minimum of 30 s, the second poll arrives within
	// ~30 s of the first (jitter can shorten it, the wait floor pins it back to 30 s). 90 s
	// gives generous headroom for jitter, scheduling, and platform variance.
	pollingRecoveryTimeout := 90 * time.Second

	// initialPollTimeout bounds the SDK's initial poll (no backoff involved; happens on client
	// construction).
	initialPollTimeout := 15 * time.Second

	// compressedExtendedInitialDelay is passed to the SDK so that the extended-regime base
	// delay is below the polling wait floor (PollInterval). With this in place the observed
	// gap between polls will be exactly PollInterval (30 s), the smallest observable window
	// for these tests.
	compressedExtendedInitialDelay := ldtime.UnixMillisecondTime(1)

	makePollEndpoint := func(t *ldtest.T, handler http.Handler) *harness.MockEndpoint {
		return requireContext(t).harness.NewMockEndpoint(handler, t.DebugLogger(),
			harness.MockEndpointDescription("polling service"))
	}

	// basePollConfig sets ExtendedInitialDelayMS to a small value so the effective retry gap
	// after an unexpected error is exactly PollInterval — the smallest observable window,
	// since the polling wait floor pins retry timing to PollInterval regardless of a smaller
	// extended base.
	basePollConfig := func(endpoint *harness.MockEndpoint) servicedef.SDKConfigPollingParams {
		return servicedef.SDKConfigPollingParams{
			BaseURI:                endpoint.BaseURL(),
			ExtendedInitialDelayMS: o.Some(compressedExtendedInitialDelay),
		}
	}

	t.Run("retry after unexpected HTTP error on initial poll", func(t *ldtest.T) {
		for _, status := range unexpectedErrors {
			t.Run(fmt.Sprintf("error %d", status), func(t *ldtest.T) {
				pollSource := NewSDKDataSourceWithoutEndpoint(t, dataV1, DataSourceOptionPolling())
				handler := httphelpers.SequentialHandler(
					httphelpers.HandlerWithStatus(status), // 1st: error
					pollSource.PollingService(),           // 2nd: success (only reached if SDK retries)
				)
				pollEndpoint := makePollEndpoint(t, handler)
				t.Defer(pollEndpoint.Close)

				client := NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{InitCanFail: true}),
					WithPollingConfig(basePollConfig(pollEndpoint)))

				// First poll -> error
				_ = pollEndpoint.RequireConnection(t, initialPollTimeout)

				// Second poll should arrive within the extended-regime window, proving the SDK
				// did not permanently stop after the unexpected error.
				_ = pollEndpoint.RequireConnection(t, pollingRecoveryTimeout)

				// RequireConnection returns as soon as the harness accepts the request; the SDK
				// still needs to read the response body, parse it, and populate its store. Poll
				// until the flag value reflects the second poll's data.
				pollUntilFlagValueUpdated(t, client, flagKey, context, ldvalue.Null(), expectedValue, ldvalue.Null())
			})
		}
	})
}
