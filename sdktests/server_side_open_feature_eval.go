package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
)

func doServerSideOpenFeatureEvalTests(t *ldtest.T) {
	expectedValueV1 := ldvalue.Int(1)
	flagKey := "flag"
	flag := ldbuilders.NewFlagBuilder(flagKey).Version(1).
		On(false).OffVariation(0).Variations(expectedValueV1).Build()
	data := mockld.NewServerSDKDataBuilder().Flag(flag).Build()
	dataSource := NewSDKDataSource(t, data)
	client := NewSDKClient(t, dataSource)

	client.EvaluateOpenFeatureFlag(t, servicedef.OpenFeatureEvaluateFlagParams{
		FlagKey:           "flag",
		EvaluationContext: map[string]string{"targetingKey": "key"},
		ValueType:         servicedef.ValueTypeInt,
		DefaultValue:      ldvalue.Int(0),
		Detail:            true,
	})
}
