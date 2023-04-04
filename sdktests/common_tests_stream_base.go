package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
)

// CommonStreamingTests groups together streaming-related test methods that are shared between server-side
// and client-side.
type CommonStreamingTests struct {
	commonTestsBase
}

func NewCommonStreamingTests(t *ldtest.T, testName string, baseSDKConfigurers ...SDKConfigurer) CommonStreamingTests {
	return CommonStreamingTests{newCommonTestsBase(t, testName, baseSDKConfigurers...)}
}

// Create a stream that can be used to push updates, and return the necessary configuration actions for
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
func (c CommonStreamingTests) setupDataSources(
	t *ldtest.T,
	initialData mockld.SDKData,
) (*SDKDataSource, []SDKConfigurer) {
	var configurers []SDKConfigurer

	streamingDataSource := NewSDKDataSource(t, initialData, DataSourceOptionStreaming())

	switch c.sdkKind {
	case mockld.ServerSideSDK:
		break

	case mockld.RokuSDK:
		fallthrough
	case mockld.MobileSDK:
		emptyPollingDataSource := NewSDKDataSource(t, nil, DataSourceOptionPolling())
		configurers = append(configurers, emptyPollingDataSource)

	case mockld.JSClientSDK:
		pollingDataSourceWithInitialData := NewSDKDataSource(t, initialData, DataSourceOptionPolling())
		configurers = append(configurers, pollingDataSourceWithInitialData)

	default:
		panic("unknown SDK kind")
	}

	return streamingDataSource, append(configurers, streamingDataSource)
}
