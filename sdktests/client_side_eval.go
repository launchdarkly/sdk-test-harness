package sdktests

import (
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"
	"github.com/launchdarkly/sdk-test-harness/testdata"
	"github.com/launchdarkly/sdk-test-harness/testdata/testmodel"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"

	"github.com/stretchr/testify/require"
)

// The tests in this file verify that the Variation and VariationDetail methods of a client-side SDK
// correctly return values that the SDK got from LaunchDarkly (or in this case our mock services).
// The only significant variables are 1. the evaluation result properties that we're feeding into the
// SDK and 2. the parameters of the evaluation method. We're not concerned with details of streaming
// or polling behavior-- those will be covered specifically in other tests-- so we just set up the
// data source to provide one initial data set.

func doClientSideEvalTests(t *ldtest.T) {
	t.Run("parameterized", runParameterizedClientSideEvalTests)
}

func runParameterizedClientSideEvalTests(t *ldtest.T) {
	// For client-side SDKs, you have to tell the SDK at initialization time whether we'll be using evaluation
	// reasons or not. The main effect of that is the client will add a withReasons query string parameter to
	// its requests-- which we're not actually checking for here; we're just configuring the polling service
	// to return the expected data based on an assumption of what the parameter is. But we're just being
	// thorough in case for some reason there's some other unexpected difference in SDK behavior depending on
	// the parameter.
	allTestSuites := getAllClientSideEvalTestSuites(t, "client-side-eval")
	for _, withReasons := range []bool{false, true} {
		t.Run(fmt.Sprintf("evaluationReasons=%t", withReasons), func(t *ldtest.T) {
			runParameterizedClientSideEvalTestsWithOrWithoutReasons(t, allTestSuites, withReasons)
		})
	}
}

func runParameterizedClientSideEvalTestsWithOrWithoutReasons(
	t *ldtest.T,
	allTestSuites []testmodel.ClientSideEvalTestSuite,
	withReasons bool,
) {
	sdkKind := helpers.IfElse(t.Capabilities().Has(servicedef.CapabilityMobile), mockld.MobileSDK, mockld.JSClientSDK)

	for _, suite := range allTestSuites {
		t.Run(suite.Name, func(t *ldtest.T) {
			if suite.RequireCapability != "" {
				t.RequireCapability(suite.RequireCapability)
			}

			data := suite.SDKData
			if !withReasons {
				data = data.WithoutReasons()
			}

			// Here we configure the data source to return an initial data set. Mobile SDKs use streaming
			// by default, and JS-based SDKs use polling by default; the simplest way to go is just to
			// tell them explicitly to both use polling. We will cover streaming behavior in other tests.
			var dataSource *SDKDataSource
			filteredData := suite.SDKData
			if !withReasons {
				filteredData = filteredData.WithoutReasons()
			}
			dataSource = NewSDKDataSource(t, filteredData, DataSourceOptionPolling(), DataSourceOptionSDKKind(sdkKind))
			client := NewSDKClient(
				t,
				WithClientSideConfig(servicedef.SDKConfigClientSideParams{
					EvaluationReasons: o.Some(withReasons),
					InitialUser:       suite.User,
				}),
				dataSource,
			)

			for _, test := range suite.Evaluations {
				name := test.Name
				if name == "" {
					name = test.FlagKey
				}
				t.Run(name, func(t *ldtest.T) {
					t.Run("evaluate flag without detail", func(t *ldtest.T) {
						params := makeClientSideEvalFlagParams(test, suite.SDKData)
						result := client.EvaluateFlag(t, params)
						m.In(t).Assert(result, EvalResponseValue().Should(m.Equal(test.Expect.Value)))
					})

					t.Run("evaluate flag with detail", func(t *ldtest.T) {
						params := makeClientSideEvalFlagParams(test, suite.SDKData)
						params.Detail = true
						result := client.EvaluateFlag(t, params)

						expectedReason := test.Expect.Reason
						if !withReasons {
							// If the client wasn't configured to request evaluation reasons, then there won't be
							// any reasons that would have had to come from LD - but we can still get an error reason
							// that is due to invalid SDK parameters.
							switch expectedReason {
							case ldreason.NewEvalReasonError(ldreason.EvalErrorFlagNotFound):
								break
							default:
								expectedReason = ldreason.EvaluationReason{}
							}
						}

						m.In(t).Assert(result, m.AllOf(
							EvalResponseValue().Should(m.Equal(test.Expect.Value)),
							EvalResponseVariation().Should(m.Equal(test.Expect.VariationIndex)),
							EvalResponseReason().Should(EqualReason(expectedReason)),
						))
					})

					if !suite.SkipEvaluateAllFlags {
						t.Run("evaluate all flags", func(t *ldtest.T) {
							result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{})
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

func getAllClientSideEvalTestSuites(t *ldtest.T, dirName string) []testmodel.ClientSideEvalTestSuite {
	sources, err := testdata.LoadAllDataFiles(dirName)
	require.NoError(t, err)

	ret := make([]testmodel.ClientSideEvalTestSuite, 0, len(sources))
	for _, source := range sources {
		suite := parseClientSideEvalTestSuite(t, source)
		ret = append(ret, suite)
	}
	return ret
}

func parseClientSideEvalTestSuite(t *ldtest.T, source testdata.SourceInfo) testmodel.ClientSideEvalTestSuite {
	var suite testmodel.ClientSideEvalTestSuite
	if err := testdata.ParseJSONOrYAML(source.Data, &suite); err != nil {
		require.NoError(t, fmt.Errorf("error parsing %q %s: %w", source.BaseName, source.ParamsString(), err))
	}
	return suite
}

func makeClientSideEvalFlagParams(
	test testmodel.ClientSideEvalTest,
	sdkData mockld.ClientSDKData,
) servicedef.EvaluateFlagParams {
	p := servicedef.EvaluateFlagParams{
		FlagKey:      test.FlagKey,
		ValueType:    test.ValueType,
		DefaultValue: test.Default,
	}
	if p.DefaultValue.IsNull() {
		p.DefaultValue = inferDefaultFromClientSideFlag(sdkData, test.FlagKey)
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
