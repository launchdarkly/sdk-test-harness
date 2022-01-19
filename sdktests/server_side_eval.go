package sdktests

import (
	"encoding/json"
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"
	"github.com/launchdarkly/sdk-test-harness/testdata"
	"github.com/launchdarkly/sdk-test-harness/testdata/testmodel"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"

	"github.com/stretchr/testify/require"
)

func DoServerSideEvalTests(t *ldtest.T) {
	RunParameterizedServerSideEvalTests(t)
	t.Run("all flags state", RunServerSideEvalAllFlagsTests)
}

func RunParameterizedServerSideEvalTests(t *ldtest.T) {
	for _, suite := range getAllServerSideEvalTestSuites(t) {
		t.Run(suite.Name, func(t *ldtest.T) {
			if suite.RequireCapability != "" {
				t.RequireCapability(suite.RequireCapability)
			}

			dataSource := NewSDKDataSource(t, suite.SDKData)
			client := NewSDKClient(t, dataSource)

			for _, test := range suite.Evaluations {
				name := test.Name
				if name == "" {
					name = test.FlagKey
				}
				t.Run(name, func(t *ldtest.T) {
					t.Run("evaluate flag without detail", func(t *ldtest.T) {
						params := makeEvalFlagParams(test, suite.SDKData)
						result := client.EvaluateFlag(t, params)
						m.AssertThat(t, result, EvalResponseValue().Should(m.Equal(test.Expect.Value)))
					})

					t.Run("evaluate flag with detail", func(t *ldtest.T) {
						params := makeEvalFlagParams(test, suite.SDKData)
						params.Detail = true
						result := client.EvaluateFlag(t, params)
						m.AssertThat(t, result, m.AllOf(
							EvalResponseValue().Should(m.Equal(test.Expect.Value)),
							EvalResponseVariation().Should(m.Equal(test.Expect.VariationIndex)),
							EvalResponseReason().Should(m.Equal(test.Expect.Reason)),
						))
					})

					if !suite.SkipEvaluateAllFlags {
						t.Run("evaluate all flags", func(t *ldtest.T) {
							result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
								User: &test.User,
							})
							if test.Expect.VariationIndex.IsDefined() {
								require.Contains(t, result.State, test.FlagKey)
							}
							expectedValue := test.Expect.Value
							if !test.Expect.VariationIndex.IsDefined() {
								expectedValue = ldvalue.Null()
							}
							m.AssertThat(t, result.State[test.FlagKey], m.Equal(expectedValue))
						})
					}
				})
			}
		})
	}
}

func getAllServerSideEvalTestSuites(t *ldtest.T) []testmodel.EvalTestSuite {
	sources, err := testdata.LoadAllDataFiles("server-side-eval")
	require.NoError(t, err)

	ret := make([]testmodel.EvalTestSuite, 0, len(sources))
	for _, source := range sources {
		suite := parseServerSideEvalTestSuite(t, source)
		ret = append(ret, suite)
	}
	return ret
}

func parseServerSideEvalTestSuite(t *ldtest.T, source testdata.SourceInfo) testmodel.EvalTestSuite {
	var suite testmodel.EvalTestSuite
	if err := testdata.ParseJSONOrYAML(source.Data, &suite); err != nil {
		require.NoError(t, fmt.Errorf("error parsing %q %s: %w", source.BaseName, source.ParamsString(), err))
	}
	return suite
}

func makeEvalFlagParams(test testmodel.EvalTest, sdkData mockld.ServerSDKData) servicedef.EvaluateFlagParams {
	p := servicedef.EvaluateFlagParams{
		FlagKey:      test.FlagKey,
		User:         &test.User,
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

func inferDefaultFromFlag(sdkData mockld.ServerSDKData, flagKey string) ldvalue.Value {
	flagData := sdkData["flags"][flagKey]
	if flagData == nil {
		return ldvalue.Null()
	}
	var flag ldmodel.FeatureFlag
	if err := json.Unmarshal(flagData, &flag); err != nil {
		return ldvalue.Null() // we should deal with malformed flag data at an earlier point
	}
	if len(flag.Variations) == 0 {
		return ldvalue.Null()
	}
	switch flag.Variations[0].Type() {
	case ldvalue.BoolType:
		return ldvalue.Bool(false)
	case ldvalue.NumberType:
		return ldvalue.Int(0)
	case ldvalue.StringType:
		return ldvalue.String("")
	default:
		return ldvalue.Null()
	}
}
