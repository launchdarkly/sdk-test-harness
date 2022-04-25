package sdktests

import "github.com/launchdarkly/sdk-test-harness/framework/ldtest"

func doClientSideTagsTests(t *ldtest.T) {
	NewClientSideTagsTests("doClientSideTagsTests").Run(t)
}
