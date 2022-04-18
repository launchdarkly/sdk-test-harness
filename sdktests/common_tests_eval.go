package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"
	"github.com/launchdarkly/sdk-test-harness/testdata"
	"github.com/launchdarkly/sdk-test-harness/testdata/testmodel"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"

	"github.com/stretchr/testify/require"
)

type CommonEvalParameterizedTestRunner[SDKDataType mockld.SDKData] struct {
	SDKConfigurers       func(testmodel.EvalTestSuite[SDKDataType]) []SDKConfigurer
	FilterSDKData        func(SDKDataType) SDKDataType
	FilterExpectedReason func(ldreason.EvaluationReason) ldreason.EvaluationReason
}

func (c CommonEvalParameterizedTestRunner[SDKDataType]) RunAll(t *ldtest.T, dirName string) {
	testSuites := testdata.LoadAndParseAllTestSuites[testmodel.EvalTestSuite[SDKDataType]](t, dirName)
	for _, suite := range testSuites {
		t.Run(suite.Name, func(t *ldtest.T) {
			if suite.RequireCapability != "" {
				t.RequireCapability(suite.RequireCapability)
			}

			sdkData := suite.SDKData
			if c.FilterSDKData != nil {
				sdkData = c.FilterSDKData(sdkData)
			}
			dataSource := NewSDKDataSource(t, sdkData)

			var clientConfig []SDKConfigurer
			if c.SDKConfigurers != nil {
				clientConfig = c.SDKConfigurers(suite)
			}
			client := NewSDKClient(t, append(clientConfig, dataSource)...)

			for _, test := range suite.Evaluations {
				name := test.Name
				if name == "" {
					name = test.FlagKey
				}
				t.Run(name, func(t *ldtest.T) {
					t.Run("evaluate flag without detail", func(t *ldtest.T) {
						params := makeEvalFlagParams(test, sdkData)
						result := client.EvaluateFlag(t, params)
						m.In(t).Assert(result, EvalResponseValue().Should(m.Equal(test.Expect.Value)))
					})

					t.Run("evaluate flag with detail", func(t *ldtest.T) {
						params := makeEvalFlagParams(test, sdkData)
						params.Detail = true
						result := client.EvaluateFlag(t, params)

						expectedReason := test.Expect.Reason
						if c.FilterExpectedReason != nil {
							expectedReason = c.FilterExpectedReason(expectedReason)
						}

						m.In(t).Assert(result, m.AllOf(
							EvalResponseValue().Should(m.Equal(test.Expect.Value)),
							EvalResponseVariation().Should(m.Equal(test.Expect.VariationIndex)),
							EvalResponseReason().Should(EqualReason(expectedReason)),
						))
					})

					if !suite.SkipEvaluateAllFlags {
						t.Run("evaluate all flags", func(t *ldtest.T) {
							result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
								User: test.User,
							})
							if test.Expect.VariationIndex.IsDefined() {
								require.Contains(t, result.State, test.FlagKey)
							}
							expectedValue := test.Expect.Value
							if !test.Expect.VariationIndex.IsDefined() {
								expectedValue = ldvalue.Null()
							}
							m.In(t).Assert(result.State[test.FlagKey], m.Equal(expectedValue))
						})
					}
				})
			}
		})
	}
}

func makeEvalFlagParams(test testmodel.EvalTest, sdkData mockld.SDKData) servicedef.EvaluateFlagParams {
	p := servicedef.EvaluateFlagParams{
		FlagKey:      test.FlagKey,
		User:         test.User,
		ValueType:    test.ValueType,
		DefaultValue: test.Default,
	}
	if p.DefaultValue.IsNull() {
		p.DefaultValue = inferDefaultFromFlag(sdkData, test.FlagKey)
	}
	if test.ValueType == "" {
		switch p.DefaultValue.Type() {
		case ldvalue.BoolType:
			p.ValueType = servicedef.ValueTypeBool
		case ldvalue.NumberType:
			if test.Default.IsInt() {
				p.ValueType = servicedef.ValueTypeInt
			} else {
				p.ValueType = servicedef.ValueTypeDouble
			}
		case ldvalue.StringType:
			p.ValueType = servicedef.ValueTypeString
		default:
			p.ValueType = servicedef.ValueTypeAny
		}
	}
	return p
}
