package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
)

func doServerSideEvalTests(t *ldtest.T) {
	t.Run("parameterized", runParameterizedServerSideEvalTests)
	t.Run("bucketing", runServerSideEvalBucketingTests)
	t.Run("all flags state", runServerSideEvalAllFlagsTests)
	t.Run("client not ready", runParameterizedServerSideClientNotReadyEvalTests)
}

func runParameterizedServerSideEvalTests(t *ldtest.T) {
	parameterizedTests := CommonEvalParameterizedTestRunner[mockld.ServerSDKData]{}
	parameterizedTests.RunAll(t, "server-side-eval")
}

func runParameterizedServerSideClientNotReadyEvalTests(t *ldtest.T) {
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
