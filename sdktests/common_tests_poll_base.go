package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
)

// CommonPollingTests groups together polling-related test methods that are shared between server-side
// and client-side.
//
// Most tests only cover the behavior of the initial poll request. Long-running tests that verify
// repeated polling at the configured interval (default 30s, minimum clamp, custom 60s) are in
// common_tests_poll_interval.go and require -enable-long-running-tests.
type CommonPollingTests struct {
	commonTestsBase
}

func NewCommonPollingTests(t *ldtest.T, testName string, baseSDKConfigurers ...SDKConfigurer) CommonPollingTests {
	return CommonPollingTests{newCommonTestsBase(t, testName, baseSDKConfigurers...)}
}
