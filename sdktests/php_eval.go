package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
)

func doPHPEvalTests(t *ldtest.T) {
	// There's no special PHP version of the evaluation tests - we're just calling the regular
	// server-side versions of those tests. The special behavior that applies for PHP is in
	// how we set up the polling endpoints in the mock data source, and that is handled
	// transparently by the logic in mockld/polling_service.go based on the fact that the
	// "SDK kind" configured for test suite is PHP.
	// t.Run("parameterized", runParameterizedServerSideEvalTests)
	t.Run("bucketing", runServerSideEvalBucketingTests)
	t.Run("all flags state", runServerSideEvalAllFlagsTests)

	t.Run("client not ready", runParameterizedServerSideClientNotReadyEvalTests)
}
