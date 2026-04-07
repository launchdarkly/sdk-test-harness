package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
)

// CommonStreamingTests groups together streaming-related test methods that are shared between server-side
// and client-side.
type CommonStreamingTests struct {
	commonTestsBase
}

func NewCommonStreamingTests(t *ldtest.T, testName string, baseSDKConfigurers ...SDKConfigurer) CommonStreamingTests {
	return CommonStreamingTests{newCommonTestsBase(t, testName, baseSDKConfigurers...)}
}

// Create a data system that can be used to push updates, and return the necessary configuration actions for
// creating an SDK client.
//
// This behavior differs between SDK types as follows:
//
// - Server-side SDKs in streaming mode use *only* the streaming service.
//
// - Mobile SDKs in streaming mode use the streaming service as their primary data source, but also need to
// have a polling service available; the polling service won't be used in these tests, we just need to be
// able to tell the SDK where it is.
//
// - JS-based client-side SDKs in streaming mode always connect to the *polling* service first for their
// initial data, and then connect to the streaming service for updates.
func (c CommonStreamingTests) setupDataSystems(
	t *ldtest.T, initialData mockld.SDKData) (*SDKDataSystem, []SDKConfigurer) {
	if initialData == nil {
		initialData = mockld.EmptyData(c.sdkKind)
	}

	if d, ok := initialData.(mockld.ServerSDKData); ok {
		initialData = d.ConvertToFDv2SDKData(t)
	}

	var dsOptions []SDKDataSystemOption

	switch c.sdkKind {
	case mockld.ServerSideSDK:
		// Streaming tests need only a streaming synchronizer (no polling initializer)
		// so the SDK connects to streaming with an empty basis. Tests that need a
		// polling initializer create their own data system via NewSDKDataSystemCustom.
		dataSystem := NewSDKDataSystemCustom(t, initialData, DataSystemOptionStreaming())
		dataSystem.CreateEndpoints()
		return dataSystem, []SDKConfigurer{dataSystem}

	case mockld.RokuSDK, mockld.MobileSDK:
		dsOptions = append(dsOptions,
			DataSystemOptionConnectionMode("streaming", DataSystemOptionStreaming()),
			DataSystemOptionConnectionMode("polling", DataSystemOptionPolling()),
			DataSystemOptionInitialConnectionMode("streaming"),
		)

	case mockld.JSClientSDK:
		// JS client-side SDKs use connection modes. Streaming-only (no polling
		// initializer) so the SDK connects to streaming with an empty basis,
		// matching the server-side behavior for these tests.
		dsOptions = append(dsOptions,
			DataSystemOptionConnectionMode("streaming", DataSystemOptionStreaming()),
			DataSystemOptionInitialConnectionMode("streaming"),
		)

	default:
		panic("unknown SDK kind")
	}

	dataSystem := NewSDKDataSystem(t, initialData, dsOptions...)
	return dataSystem, []SDKConfigurer{dataSystem}
}
