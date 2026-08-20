package sdktests

import (
	"time"

	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/sdk-test-harness/v3/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"
)

const pollIntervalEpsilon = 2 * time.Second

// pollIntervals returns the default and minimum values used for interval testing.
// Uses server-side/client-side capabilities; uses sdkKind only for the browser (JSClientSDK) special case.
// Panics if no condition matches. Return type is an anonymous struct so it is not referenced from other files.
func (c CommonPollingTests) pollIntervals(t *ldtest.T) struct{ Default, Min time.Duration } {
	if t.Capabilities().Has(servicedef.CapabilityServerSide) {
		return struct{ Default, Min time.Duration }{Default: 30 * time.Second, Min: 30 * time.Second}
	}
	// Browser (JS) SDKs use 5min default, 30s min.
	if c.sdkKind == mockld.JSClientSDK {
		return struct{ Default, Min time.Duration }{Default: 5 * time.Minute, Min: 30 * time.Second}
	}
	if t.Capabilities().Has(servicedef.CapabilityClientSide) {
		return struct{ Default, Min time.Duration }{Default: 5 * time.Minute, Min: 5 * time.Minute}
	}
	require.Fail(t, "pollIntervals: test service has neither server-side nor client-side capability")
	return struct{ Default, Min time.Duration }{} // unreachable; satisfy compiler
}

// PollingIntervalTests requires -enable-long-running-tests and only runs when client-side or
// server-side-polling capability is present.
func (c CommonPollingTests) PollingIntervalTests(t *ldtest.T) {
	if t.Capabilities().HasAny(servicedef.CapabilityClientSide, servicedef.CapabilityServerSidePolling) {
		t.LongRunning()
		t.Run("default polling interval is respected", c.defaultPollingIntervalIsRespected)
		t.Run("polling interval below minimum is clamped", c.pollingIntervalBelowMinimumIsClamped)
		t.Run("polling interval above minimum is respected", c.pollingIntervalAboveMinimumIsRespected)
	}
}

// doPollIntervalTests runs the polling interval tests (default, clamp, custom). Used by both
// server-side and client-side poll suites; pass the appropriate SDKConfigurers for the context.
func doPollIntervalTests(t *ldtest.T, baseSDKConfigurers ...SDKConfigurer) {
	pollTests := NewCommonPollingTests(t, "pollInterval", baseSDKConfigurers...)
	pollTests.PollingIntervalTests(t)
}

func (c CommonPollingTests) defaultPollingIntervalIsRespected(t *ldtest.T) {
	t.LongRunning()

	dataSystem := NewSDKDataSystem(t, nil, DataSystemOptionPolling())
	_ = NewSDKClient(t, c.baseSDKConfigurationPlus(dataSystem)...)

	// Do not set PollIntervalMS; SDK uses its default (30s server-side, 5min client-side including browser).
	intervals := c.pollIntervals(t)
	c.assertPollingIntervalBetweenRequests(t, dataSystem.Synchronizers[0].Endpoint(),
		intervals.Default, pollIntervalEpsilon)
}

func (c CommonPollingTests) pollingIntervalBelowMinimumIsClamped(t *ldtest.T) {
	t.LongRunning()

	// Request below minimum (min/2); SDK should clamp to minimum (30s server-side and browser, 5min other client-side).
	params := c.pollIntervals(t)
	dataSystem := NewSDKDataSystem(t, nil, DataSystemOptionPolling(), DataSystemOptionPollInterval(params.Min/2))
	_ = NewSDKClient(t, c.baseSDKConfigurationPlus(dataSystem)...)

	c.assertPollingIntervalBetweenRequests(t, dataSystem.Synchronizers[0].Endpoint(), params.Min, pollIntervalEpsilon)
}

func (c CommonPollingTests) pollingIntervalAboveMinimumIsRespected(t *ldtest.T) {
	t.LongRunning()

	params := c.pollIntervals(t)
	// Use default*1.5 as a configured interval above minimum to verify it is respected.
	customInterval := params.Default * 3 / 2
	dataSystem := NewSDKDataSystem(t, nil, DataSystemOptionPolling(), DataSystemOptionPollInterval(customInterval))
	_ = NewSDKClient(t, c.baseSDKConfigurationPlus(dataSystem)...)

	endpoint := dataSystem.Synchronizers[0].Endpoint()

	// First request (initial poll). Use same generous timeout as assertPollingIntervalBetweenRequests for SDK startup.
	_ = endpoint.RequireConnection(t, time.Second*15)
	firstReceivedAt := time.Now()

	// Second request must not occur before the configured interval. Wait slightly less and assert no request.
	noRequestBefore := customInterval - pollIntervalEpsilon
	endpoint.RequireNoMoreConnections(t, noRequestBefore)

	// Now the second request should arrive within the next few seconds (interval + epsilon). Match helper's +5s buffer.
	_ = endpoint.RequireConnection(t, pollIntervalEpsilon+5*time.Second)
	secondReceivedAt := time.Now()

	elapsed := secondReceivedAt.Sub(firstReceivedAt)
	minAllowed := customInterval - pollIntervalEpsilon
	if elapsed < minAllowed {
		t.Errorf("second polling request arrived after %v; expected at least %v (configured interval %v)",
			elapsed, minAllowed, customInterval)
	}
}

// assertPollingIntervalBetweenRequests waits for two polling requests and asserts the time between
// them is within epsilon of the expected interval.
func (c CommonPollingTests) assertPollingIntervalBetweenRequests(
	t *ldtest.T,
	endpoint *harness.MockEndpoint,
	expectedInterval time.Duration,
	epsilon time.Duration,
) {
	// Allow generous timeout for first request (SDK startup).
	_ = endpoint.RequireConnection(t, time.Second*15)
	firstReceivedAt := time.Now()

	// Second request: expect it after expectedInterval ± epsilon.
	_ = endpoint.RequireConnection(t, expectedInterval+epsilon+5*time.Second)
	secondReceivedAt := time.Now()

	elapsed := secondReceivedAt.Sub(firstReceivedAt)
	minAllowed := expectedInterval - epsilon
	maxAllowed := expectedInterval + epsilon
	if elapsed < minAllowed || elapsed > maxAllowed {
		t.Errorf("second polling request arrived after %v; expected between %v and %v", elapsed, minAllowed, maxAllowed)
	}
}
