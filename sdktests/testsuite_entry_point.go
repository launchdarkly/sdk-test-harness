package sdktests

import (
	"errors"
	"fmt"
	"os"

	"github.com/launchdarkly/sdk-test-harness/v2/framework"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

func RunSDKTestSuite(
	harness *harness.TestHarness,
	filter ldtest.Filter,
	testLogger ldtest.TestLogger,
) ldtest.Results {
	capabilities := harness.TestServiceInfo().Capabilities
	var importantCapabilities framework.Capabilities
	var sdkKind mockld.SDKKind

	switch {
	case capabilities.Has(servicedef.CapabilityServerSide) && capabilities.Has(servicedef.CapabilityPHP):
		fmt.Println("Running PHP SDK test suite")
		sdkKind = mockld.PHPSDK
	case capabilities.Has(servicedef.CapabilityServerSide):
		fmt.Println("Running server-side SDK test suite")
		sdkKind = mockld.ServerSideSDK
		importantCapabilities = allImportantServerSideCapabilities()
	case capabilities.Has(servicedef.CapabilityClientSide) && capabilities.Has(servicedef.CapabilityMobile):
		fmt.Println("Running client-side (mobile) SDK test suite")
		sdkKind = mockld.MobileSDK
	case capabilities.Has(servicedef.CapabilityClientSide):
		fmt.Println("Running client-side (JS) SDK test suite")
		sdkKind = mockld.JSClientSDK
	default:
		return ldtest.Results{
			Failures: []ldtest.TestResult{
				{
					Errors: []error{
						errors.New(`test service has neither "client-side" nor "server-side" capability`),
					},
				},
			},
		}
	}

	fmt.Println()
	if sdf, ok := filter.(ldtest.SelfDescribingFilter); ok {
		sdf.Describe(os.Stdout, capabilities, importantCapabilities)
	}

	config := ldtest.TestConfiguration{
		Filter:       filter,
		Capabilities: harness.TestServiceInfo().Capabilities,
		TestLogger:   testLogger,
		Context: SDKTestContext{
			harness: harness,
			sdkKind: sdkKind,
		},
	}

	return ldtest.Run(config, func(t *ldtest.T) {
		switch sdkKind {
		case mockld.ServerSideSDK:
			doAllServerSideTests(t)
		case mockld.PHPSDK:
			doAllPHPTests(t)
		default:
			doAllClientSideTests(t)
		}
	})
}

func doAllServerSideTests(t *ldtest.T) {
	t.Run("evaluation", doServerSideEvalTests)
	t.Run("events", doServerSideEventTests)
	t.Run("streaming", doServerSideStreamTests)
	t.Run("polling", doServerSidePollTests)
	t.Run("big segments", doServerSideBigSegmentsTests)
	t.Run("service endpoints", doServerSideServiceEndpointsTests)
	t.Run("tags", doServerSideTagsTests)
	t.Run("secure mode hash", doServerSideSecureModeHashTests)
	t.Run("context type", doSDKContextTypeTests)
}

func doAllClientSideTests(t *ldtest.T) {
	t.Run("evaluation", doClientSideEvalTests)
	t.Run("events", doClientSideEventTests)
	t.Run("streaming", doClientSideStreamTests)
	t.Run("polling", doClientSidePollTests)
	t.Run("tags", doClientSideTagsTests)
	t.Run("context type", doSDKContextTypeTests)
}

func doAllPHPTests(t *ldtest.T) {
	t.Run("evaluation", doPHPEvalTests)
}

func allImportantServerSideCapabilities() framework.Capabilities {
	return framework.Capabilities{
		servicedef.CapabilityAllFlagsClientSideOnly,
		servicedef.CapabilityAllFlagsDetailsOnlyForTrackedFlags,
		servicedef.CapabilityAllFlagsWithReasons,
		servicedef.CapabilityBigSegments,
	}
	// We don't include the "strongly-typed" capability here because it's not unusual for an SDK
	// to not have it - that's just an inherent characteristic of the SDK, not a missing feature
}
