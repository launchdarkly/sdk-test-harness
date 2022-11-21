package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/data/testmodel"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/require"
)

type CommonEvalParameterizedTestRunner[SDKDataType mockld.SDKData] struct {
	SDKConfigurers       func(testmodel.EvalTestSuite[SDKDataType]) []SDKConfigurer
	FilterSDKData        func(SDKDataType) SDKDataType
	FilterExpectedReason func(ldreason.EvaluationReason) ldreason.EvaluationReason
}

func (c CommonEvalParameterizedTestRunner[SDKDataType]) RunAll(t *ldtest.T, dirName string) {
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

	testSuites := data.LoadAndParseAllTestSuites[testmodel.EvalTestSuite[SDKDataType]](t, dirName)
	groups := data.GroupTestSuitesByName(testSuites)

	for _, group := range groups {
		if len(group) == 1 {
			t.Run(group[0].Name, func(t *ldtest.T) {
				c.runTestSuite(t, group[0])
			})
		} else {
			t.Run(group[0].Name, func(t *ldtest.T) {
				for _, suite := range group {
					c.runTestSuite(t, suite)
				}
			})
		}
	}
}

func (c CommonEvalParameterizedTestRunner[SDKDataType]) runTestSuite(
	t *ldtest.T,
	suite testmodel.EvalTestSuite[SDKDataType],
) {
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

	// We can't rely on the test framework's usual auto-closing of the client, because this method could
	// be called multiple times for a single value of t.
	defer func() {
		_ = client.Close()
	}()

	if len(suite.Evaluations) == 1 && suite.Evaluations[0].Name == "" {
		c.runTestEval(t, suite, suite.Evaluations[0], sdkData, client)
	} else {
		for _, test := range suite.Evaluations {
			name := test.Name
			if name == "" {
				name = test.FlagKey
			}
			t.Run(name, func(t *ldtest.T) {
				c.runTestEval(t, suite, test, sdkData, client)
			})
		}
	}
}

func (c CommonEvalParameterizedTestRunner[SDKDataType]) runTestEval(
	t *ldtest.T,
	suite testmodel.EvalTestSuite[SDKDataType],
	test testmodel.EvalTest,
	sdkData SDKDataType,
	client *SDKClient,
) {
	name := test.Name
	if name == "" {
		name = test.FlagKey
	}

	// *If* the context for this test can be represented in the old user model, then we will also do
	// the test with an equivalent old-style user representation.
	user := representContextAsOldUser(t, test.Context.Value())

	t.Run(name, func(t *ldtest.T) {
		t.Run("evaluate flag without detail", func(t *ldtest.T) {
			params := makeEvalFlagParams(test, sdkData)
			result := client.EvaluateFlag(t, params)
			m.In(t).Assert(result, EvalResponseValue().Should(m.Equal(test.Expect.Value)))

			if user != nil {
				params.User = user
				params.Context = o.None[ldcontext.Context]()
				t.Run("with old user", func(t *ldtest.T) {
					result := client.EvaluateFlag(t, params)
					m.In(t).Assert(result, EvalResponseValue().Should(m.Equal(test.Expect.Value)))
				})
			}
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

			if user != nil {
				params.User = user
				params.Context = o.None[ldcontext.Context]()
				t.Run("with old user", func(t *ldtest.T) {
					result := client.EvaluateFlag(t, params)
					m.In(t).Assert(result, m.AllOf(
						EvalResponseValue().Should(m.Equal(test.Expect.Value)),
						EvalResponseVariation().Should(m.Equal(test.Expect.VariationIndex)),
						EvalResponseReason().Should(EqualReason(expectedReason)),
					))
				})
			}
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

				if user != nil {
					t.Run("with old user", func(t *ldtest.T) {
						result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{User: user})
						if test.Expect.VariationIndex.IsDefined() {
							require.Contains(t, result.State, test.FlagKey)
						}
						m.In(t).Assert(result.State[test.FlagKey], m.Equal(expectedValue))
					})
				}
			})
		}
	})
}

func makeEvalFlagParams(test testmodel.EvalTest, sdkData mockld.SDKData) servicedef.EvaluateFlagParams {
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
