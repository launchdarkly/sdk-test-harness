package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

const defaultEventTimeout = time.Second * 5

func AllImportantServerSideCapabilities() framework.Capabilities {
	return framework.Capabilities{
		servicedef.CapabilityAllFlagsClientSideOnly,
		servicedef.CapabilityAllFlagsDetailsOnlyForTrackedFlags,
		servicedef.CapabilityAllFlagsWithReasons,
		servicedef.CapabilityBigSegments,
	}
	// We don't include the "strongly-typed" capability here because it's not unusual for an SDK
	// to not have it - that's just an inherent characteristic of the SDK, not a missing feature
}

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
		t.Run("big segments", doServerSideBigSegmentsTests)
		t.Run("tags", doServerSideTagsTests)
		t.Run("context type", doSDKContextTypeTests)
	})
}

func doServerSideBigSegmentsTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityBigSegments)

	t.Run("evaluation", doBigSegmentsEvaluateSegment)
	t.Run("membership caching", doBigSegmentsMembershipCachingTests)
	t.Run("status polling", doBigSegmentsStatusPollingTests)
	t.Run("error handling", doBigSegmentsErrorHandlingTests)
}

func doServerSideDataStoreTests(t *ldtest.T) {
	t.Run("updates from stream", doServerSideDataStoreStreamUpdateTests)
}

func doServerSideEventTests(t *ldtest.T) {
	t.Run("summary events", doServerSideSummaryEventTests)
	t.Run("feature events", doServerSideFeatureEventTests)
	t.Run("debug events", doServerSideDebugEventTests)
	t.Run("feature prerequisite events", doServerSideFeaturePrerequisiteEventTests)
	t.Run("experimentation", doServerSideExperimentationEventTests)
	t.Run("identify events", doServerSideIdentifyEventTests)
	t.Run("custom events", doServerSideCustomEventTests)
	t.Run("index events", doServerSideIndexEventTests)
	t.Run("user properties", doServerSideEventUserTests)
	t.Run("event capacity", doServerSideEventBufferTests)
}

func doServerSideStreamTests(t *ldtest.T) {
	t.Run("requests", doServerSideStreamRequestTests)
	t.Run("retry behavior", doServerSideStreamRetryTests)
	t.Run("validation", doServerSideStreamValidationTests)
}
