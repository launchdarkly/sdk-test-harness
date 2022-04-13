package sdktests

import (
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"
	"github.com/launchdarkly/sdk-test-harness/testdata"
	"github.com/launchdarkly/sdk-test-harness/testdata/testmodel"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"

	"github.com/stretchr/testify/require"
)

func DoServerSideEvalTests(t *ldtest.T) {
	t.Run("parameterized", RunParameterizedServerSideEvalTests)
	t.Run("bucketing", RunServerSideEvalBucketingTests)
	t.Run("all flags state", RunServerSideEvalAllFlagsTests)
	t.Run("client not ready", RunParameterizedServerSideClientNotReadyEvalTests)
}

func RunParameterizedServerSideEvalTests(t *ldtest.T) {
	for _, suite := range getAllServerSideEvalTestSuites(t, "server-side-eval") {
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
						m.In(t).Assert(result, EvalResponseValue().Should(m.Equal(test.Expect.Value)))
					})

					t.Run("evaluate flag with detail", func(t *ldtest.T) {
						params := makeEvalFlagParams(test, suite.SDKData)
						params.Detail = true
						result := client.EvaluateFlag(t, params)
						m.In(t).Assert(result, m.AllOf(
							EvalResponseValue().Should(m.Equal(test.Expect.Value)),
							EvalResponseVariation().Should(m.Equal(optionalIntToMaybe(test.Expect.VariationIndex))),
							EvalResponseReason().Should(EqualReason(test.Expect.Reason)),
						))
					})

					if !suite.SkipEvaluateAllFlags {
						t.Run("evaluate all flags", func(t *ldtest.T) {
							result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
								User: o.Some(test.User),
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

func RunParameterizedServerSideClientNotReadyEvalTests(t *ldtest.T) {
	defaultValues := DefaultValueByTypeFactory()
	flagKey := "some-flag"
	user := lduser.NewUser("user-key")
	expectedReason := ldreason.NewEvalReasonError(ldreason.EvalErrorClientNotReady)

	dataSource := NewSDKDataSource(t, mockld.BlockingUnavailableSDKData(mockld.ServerSideSDK))
	client := NewSDKClient(t,
		WithConfig(servicedef.SDKConfigParams{StartWaitTimeMS: o.Some(ldtime.UnixMillisecondTime(1)),
			InitCanFail: true}),
		dataSource)

	for _, valueType := range getValueTypesToTest(t) {
		t.Run(testDescFromType(valueType), func(t *ldtest.T) {
			defaultValue := defaultValues(valueType)

			t.Run("evaluate flag without detail", func(t *ldtest.T) {
				result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
					FlagKey:      flagKey,
					User:         o.Some(user),
					ValueType:    valueType,
					DefaultValue: defaultValue,
				})
				m.In(t).Assert(result, EvalResponseValue().Should(m.JSONEqual(defaultValue)))
			})

			t.Run("evaluate flag with detail", func(t *ldtest.T) {
				result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
					FlagKey:      flagKey,
					User:         o.Some(user),
					ValueType:    valueType,
					DefaultValue: defaultValue,
					Detail:       true,
				})
				m.In(t).Assert(result, m.AllOf(
					EvalResponseValue().Should(m.Equal(defaultValue)),
					EvalResponseVariation().Should(m.Equal(o.None[int]())),
					EvalResponseReason().Should(EqualReason(expectedReason)),
				))
			})
		})
	}
}

func getAllServerSideEvalTestSuites(t *ldtest.T, dirName string) []testmodel.EvalTestSuite {
	sources, err := testdata.LoadAllDataFiles(dirName)
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
		User:         o.Some(test.User),
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
