package sdktests

import (
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/v2/data/testmodel"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
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
	for _, withReasons := range []bool{false, true} {
		t.Run(fmt.Sprintf("evaluationReasons=%t", withReasons), func(t *ldtest.T) {
			parameterizedTests := CommonEvalParameterizedTestRunner[mockld.ClientSDKData]{
				SDKConfigurers: func(testSuite testmodel.EvalTestSuite[mockld.ClientSDKData]) []SDKConfigurer {
					if !testSuite.Context.IsDefined() {
						t.Errorf("client-side test suite %q did not define a context", testSuite.Name)
						t.FailNow()
					}
					return []SDKConfigurer{
						WithClientSideConfig(servicedef.SDKConfigClientSideParams{
							EvaluationReasons: o.Some(withReasons),
							InitialContext:    testSuite.Context.Value(),
						}),
					}
				},
				FilterSDKData: func(data mockld.ClientSDKData) mockld.ClientSDKData {
					if !withReasons {
						return data.WithoutReasons()
					}
					return data
				},
				FilterExpectedReason: func(reason ldreason.EvaluationReason) ldreason.EvaluationReason {
					if withReasons {
						return reason
					}
					// If the client wasn't configured to request evaluation reasons, then there won't be
					// any reasons that would have had to come from LD - but we can still get an error reason
					// that is due to invalid SDK parameters.
					switch reason {
					case ldreason.NewEvalReasonError(ldreason.EvalErrorFlagNotFound),
						ldreason.NewEvalReasonError(ldreason.EvalErrorWrongType):
						return reason
					}
					return ldreason.EvaluationReason{}
				},
			}
			parameterizedTests.RunAll(t, "client-side-eval")
		})
	}
}
