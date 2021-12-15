package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
)

func RunServerSideTestSuite(
	harness *harness.TestHarness,
	filter ldtest.Filter,
	testLogger ldtest.TestLogger,
) ldtest.Results {
	config := ldtest.TestConfiguration{
		Filter:       filter,
		Capabilities: harness.TestServiceInfo().Capabilities,
		TestLogger:   testLogger,
		Context: SDKTestContext{
			harness: harness,
			sdkKind: mockld.ServerSideSDK,
		},
	}

	return ldtest.Run(config, func(t *ldtest.T) {
		//nolint:godox
		// TODO
		// t.Run("evaluation", DoServerSideEvalTests)
		// t.Run("events", doServerSideEventTests)
	})
}

// func doServerSideEventTests(t *ldtest.T) {
// 	t.Run("feature events", doServerSideFeatureEventTests)
// 	t.Run("identify events", doServerSideIdentifyEventTests)
// 	t.Run("custom events", doServerSideCustomEventTests)
// 	t.Run("user properties", doServerSideEventUserTests)
// }
