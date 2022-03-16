package sdktests

import (
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/data/testmodel"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldcontext"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldvalue"

	"github.com/stretchr/testify/require"
)

func DoServerSideEvalTests(t *ldtest.T) {
	t.Run("parameterized", RunParameterizedServerSideEvalTests)
	t.Run("all flags state", RunServerSideEvalAllFlagsTests)
	t.Run("client not ready", RunParameterizedServerSideClientNotReadyEvalTests)
}

func RunParameterizedServerSideEvalTests(t *ldtest.T) {
	// We use the parameter expansion feature in the test data files in a few different ways, so for any
	// given file we might end up with inputs that look like this--
	//      test suite 1:
	//			name: "things - test case 1"
	//          sdkData: (data set 1)
	//          evaluations: (name: "a")
	//      test suite 2:
	//			name: "things - test case 2"
	//			sdkData: (data set 1)
	//          evaluations: (name: "b)
	//
	// --or, both of those test suites might have the same name, "things". In the latter case, the test
	// output looks nicer if we represent it as a parent test called "things" with subtests "a" and "b",
	// rather than two parent tests called "things" with one subtest each. So, getAllServerSideEvalTests
	// will group them together by the top-level name, and if we see more than one in a group then we
	// know to go with the latter option.
	for _, group := range getAllServerSideEvalTestSuites(t, "server-side-eval") {
		if len(group) == 1 {
			t.Run(group[0].Name, func(t *ldtest.T) {
				runParameterizedTestSuite(t, group[0])
			})
		} else {
			t.Run(group[0].Name, func(t *ldtest.T) {
				for _, suite := range group {
					runParameterizedTestSuite(t, suite)
				}
			})
		}
	}
}

func runParameterizedTestSuite(t *ldtest.T, suite testmodel.EvalTestSuite) {
	if suite.RequireCapability != "" {
		t.RequireCapability(suite.RequireCapability)
	}

	dataSource := NewSDKDataSource(t, suite.SDKData)
	client := NewSDKClient(t, dataSource)

	if len(suite.Evaluations) == 1 && suite.Evaluations[0].Name == "" {
		runParameterizedTestEval(t, suite, suite.Evaluations[0], client)
	} else {
		for _, test := range suite.Evaluations {
			name := test.Name
			if name == "" {
				name = test.FlagKey
			}
			t.Run(name, func(t *ldtest.T) {
				runParameterizedTestEval(t, suite, test, client)
			})
		}
	}
}

func runParameterizedTestEval(t *ldtest.T, suite testmodel.EvalTestSuite, test testmodel.EvalTest, client *SDKClient) {
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
			EvalResponseVariation().Should(m.Equal(test.Expect.VariationIndex)),
			EvalResponseReason().Should(m.Equal(test.Expect.Reason)),
		))
	})

	if !suite.SkipEvaluateAllFlags {
		t.Run("evaluate all flags", func(t *ldtest.T) {
			result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
				Context: test.Context,
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
}

func RunParameterizedServerSideClientNotReadyEvalTests(t *ldtest.T) {
	defaultValues := data.MakeValueFactoryBySDKValueType()
	flagKey := "some-flag"
	context := ldcontext.New("user-key")
	expectedReason := ldreason.NewEvalReasonError(ldreason.EvalErrorClientNotReady)

	dataSource := NewSDKDataSource(t, mockld.BlockingUnavailableSDKData(mockld.ServerSideSDK))
	client := NewSDKClient(t,
		WithConfig(servicedef.SDKConfigParams{StartWaitTimeMS: 1, InitCanFail: true}),
		dataSource)

	for _, valueType := range getValueTypesToTest(t) {
		t.Run(testDescFromType(valueType), func(t *ldtest.T) {
			defaultValue := defaultValues(valueType)

			t.Run("evaluate flag without detail", func(t *ldtest.T) {
				result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
					FlagKey:      flagKey,
					Context:      context,
					ValueType:    valueType,
					DefaultValue: defaultValue,
				})
				m.In(t).Assert(result, EvalResponseValue().Should(m.JSONEqual(defaultValue)))
			})

			t.Run("evaluate flag with detail", func(t *ldtest.T) {
				result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
					FlagKey:      flagKey,
					Context:      context,
					ValueType:    valueType,
					DefaultValue: defaultValue,
					Detail:       true,
				})
				m.In(t).Assert(result, m.AllOf(
					EvalResponseValue().Should(m.Equal(defaultValue)),
					EvalResponseVariation().Should(m.Equal(ldvalue.OptionalInt{})),
					EvalResponseReason().Should(m.JSONEqual(expectedReason)),
				))
			})
		})
	}
}

func getAllServerSideEvalTestSuites(t *ldtest.T, dirName string) [][]testmodel.EvalTestSuite {
	// See comments in RunParameterizedServerSideEvalTests regarding the reason for grouping the
	// results by name as we're doing here.
	sources, err := data.LoadAllDataFiles(dirName)
	require.NoError(t, err)

	ret := [][]testmodel.EvalTestSuite{}
	var curName string
	curGroup := []testmodel.EvalTestSuite{}
	for _, source := range sources {
		suite := parseServerSideEvalTestSuite(t, source)
		if suite.Name != curName {
			if curName != "" {
				ret = append(ret, curGroup)
				curGroup = []testmodel.EvalTestSuite{}
			}
			curName = suite.Name
		}
		curGroup = append(curGroup, suite)
	}
	ret = append(ret, curGroup)
	return ret
}

func parseServerSideEvalTestSuite(t *ldtest.T, source data.SourceInfo) testmodel.EvalTestSuite {
	var suite testmodel.EvalTestSuite
	if err := data.ParseJSONOrYAML(source.Data, &suite); err != nil {
		require.NoError(t, fmt.Errorf("error parsing %q %s: %w", source.BaseName, source.ParamsString(), err))
	}
	return suite
}

func makeEvalFlagParams(test testmodel.EvalTest, sdkData mockld.ServerSDKData) servicedef.EvaluateFlagParams {
	p := servicedef.EvaluateFlagParams{
		FlagKey:      test.FlagKey,
		Context:      test.Context,
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
