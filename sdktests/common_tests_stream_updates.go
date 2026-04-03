package sdktests

import (
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

func (c CommonStreamingTests) makeSDKDataWithFlag(version int, value ldvalue.Value) mockld.SDKData {
	if c.isClientSide {
		cd := mockld.NewClientSDKDataBuilder().
			Flag("flag-key", c.makeClientSideFlag("flag-key", version, value).ClientSDKFlag).
			Build()
		return mockld.FDv2SDKDataFromClientSDKData(cd, "xfer-full", "initial", "initial")
	}
	sd := mockld.NewServerSDKDataBuilder().
		Flag(c.makeServerSideFlag("flag-key", version, value)).
		Build()
	return mockld.FDv2SDKDataFromServerSDKData(sd, "xfer-full", "initial", "initial")
}

// fdv2ServerData builds FDv2 mock data from server-side flags with an explicit envelope.
func (c CommonStreamingTests) fdv2ServerData(intentCode, intentReason, state string, flags ...ldmodel.FeatureFlag) mockld.FDv2SDKData {
	b := mockld.NewServerSDKDataBuilder()
	if len(flags) > 0 {
		b.Flag(flags...)
	}
	return mockld.FDv2SDKDataFromServerSDKData(b.Build(), intentCode, intentReason, state)
}

// fdv2ClientData builds FDv2 mock data from client-side flag eval results with an explicit envelope.
func (c CommonStreamingTests) fdv2ClientData(intentCode, intentReason, state string, flags ...mockld.ClientSDKFlagWithKey) mockld.FDv2SDKData {
	b := mockld.NewClientSDKDataBuilder()
	for _, f := range flags {
		b.FullFlag(f)
	}
	return mockld.FDv2SDKDataFromClientSDKData(b.Build(), intentCode, intentReason, state)
}

func (c CommonStreamingTests) newFDv2SDKClient(t *ldtest.T, configurers ...SDKConfigurer) *SDKClient {
	if c.isClientSide {
		return NewSDKClient(t, append([]SDKConfigurer{WithClientSideInitialContext(fdv2StreamingTestContext)}, configurers...)...)
	}
	return NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)
}

func (c CommonStreamingTests) makeFlagData(key string, version int, value ldvalue.Value) []byte {
	if c.isClientSide {
		return jsonhelpers.ToJSON(c.makeClientSideFlag(key, version, value))
	}
	return jsonhelpers.ToJSON(c.makeServerSideFlag(key, version, value))
}

func (c CommonStreamingTests) makeClientSideFlag(
	key string,
	version int,
	value ldvalue.Value,
) mockld.ClientSDKFlagWithKey {
	return mockld.ClientSDKFlagWithKey{
		Key: key,
		ClientSDKFlag: mockld.ClientSDKFlag{
			Version: version,
			Value:   value,
		},
	}
}

func (c CommonStreamingTests) makeServerSideFlag(key string, version int, value ldvalue.Value) ldmodel.FeatureFlag {
	return ldbuilders.NewFlagBuilder(key).Version(version).
		On(false).OffVariation(0).Variations(value, ldvalue.String("other")).
		Build()
}
