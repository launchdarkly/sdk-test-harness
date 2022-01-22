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
		t.Run("data store", doServerSideDataStoreTests)
		t.Run("evaluation", DoServerSideEvalTests)
		t.Run("events", doServerSideEventTests)
		t.Run("streaming", doServerSideStreamTests)
	})
}

func doServerSideDataStoreTests(t *ldtest.T) {
	t.Run("updates from stream", doServerSideDataStoreStreamUpdateTests)
}

func doServerSideEventTests(t *ldtest.T) {
	t.Run("summary events", doServerSideSummaryEventTests)
	t.Run("feature events", doServerSideFeatureEventTests)
	t.Run("feature prerequisite events", doServerSideFeaturePrerequisiteEventTests)
	t.Run("identify events", doServerSideIdentifyEventTests)
	t.Run("custom events", doServerSideCustomEventTests)
	t.Run("alias events", doServerSideAliasEventTests)
	t.Run("user properties", doServerSideEventUserTests)
	t.Run("event capacity", doServerSideEventBufferTests)
}

func doServerSideStreamTests(t *ldtest.T) {
	t.Run("requests", doServerSideStreamRequestTests)
	t.Run("retry behavior", doServerSideStreamRetryTests)
	t.Run("validation", doServerSideStreamValidationTests)
}
