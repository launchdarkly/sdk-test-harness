package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
)

func doServerSideTagsTests(t *ldtest.T) {
	NewCommonTagsTests(t, "doServerSideTagsTests").Run(t)
}
