package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
)

const defaultEventTimeout = time.Second * 5

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
		t.Run("evaluation", DoServerSideEvalTests)
		t.Run("events", doServerSideEventTests)
	})
}

func doServerSideEventTests(t *ldtest.T) {
	t.Run("feature events", doServerSideFeatureEventTests)
	t.Run("identify events", doServerSideIdentifyEventTests)
	t.Run("custom events", doServerSideCustomEventTests)
	t.Run("user properties", doServerSideEventUserTests)
}
