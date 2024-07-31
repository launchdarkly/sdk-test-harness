package sdktests

import "github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"

func doServerSideWrapperTests(t *ldtest.T) {
	NewCommonWrapperTests(t, "doServerSideWrapperTests").Run(t)
}
