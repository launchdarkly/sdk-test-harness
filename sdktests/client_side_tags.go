package sdktests

import "github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"

func doClientSideTagsTests(t *ldtest.T) {
	NewCommonTagsTests(t, "doClientSideTagsTests").Run(t)
}
