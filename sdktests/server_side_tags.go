package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
)

func doServerSideTagsTests(t *ldtest.T) {
	NewCommonTagsTests(t, "doServerSideTagsTests").Run(t)
}
