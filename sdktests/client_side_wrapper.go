package sdktests

import "github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"

func doClientSideWrapperTests(t *ldtest.T) {
	NewCommonWrapperTests(t, "doClientSideWrapperTests").Run(t)
}
