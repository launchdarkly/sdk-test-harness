package sdktests

import "github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"

func doClientSideWrapperTests(t *ldtest.T) {
	NewCommonWrapperTests(t, "doClientSideWrapperTests").Run(t)
}
