package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
)

// CommonPollingTests groups together polling-related test methods that are shared between server-side
// and client-side.
//
// Currently we do not have any tests that actually test *repeated* polling. This is because the SDKs
// enforce minimum polling intervals that would cause the tests to take a very long time. Therefore,
// the current tests only cover the behavior of the initial poll request.
type CommonPollingTests struct {
	commonTestsBase
}

func NewCommonPollingTests(t *ldtest.T, testName string, baseSDKConfigurers ...SDKConfigurer) CommonPollingTests {
	return CommonPollingTests{newCommonTestsBase(t, testName, baseSDKConfigurers...)}
}
