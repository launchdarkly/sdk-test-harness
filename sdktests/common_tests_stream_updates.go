package sdktests

import (
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

func (c CommonStreamingTests) makeSDKDataWithFlag(version int, value ldvalue.Value) mockld.SDKData {
	if c.isClientSide {
		return mockld.NewClientSDKDataBuilder().
			Flag("flag-key", c.makeClientSideFlag("flag-key", version, value).ClientSDKFlag).
			Build()
	}
	return mockld.NewServerSDKDataBuilder().Flag(c.makeServerSideFlag("flag-key", version, value)).Build()
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
