package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
)

func doAllClientSideTests(t *ldtest.T) {
	t.Run("evaluation", doClientSideEvalTests)
}
