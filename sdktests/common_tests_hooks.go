package sdktests

import (
	"strconv"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldmigration"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"

	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

func doCommonHooksTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityEvaluationHooks)
	t.Run("executes beforeEvaluation stage", executesBeforeEvaluationStage)
	t.Run("executes afterEvaluation stage", executesAfterEvaluationStage)
	t.Run("an error in before stage does not affect after stage", errorInBeforeStageDoesNotAffectAfterStage)

	t.Run("data propagates from before to after", beforeEvaluationDataPropagatesToAfter)
	t.RequireCapability(servicedef.CapabilityMigrations)
	t.Run("data propagates from before to after for migrations", beforeEvaluationDataPropagatesToAfterMigration)
}

func executesBeforeEvaluationStage(t *ldtest.T) {
	t.Run("without detail", func(t *ldtest.T) { executesBeforeEvaluationStageDetail(t, false) })
	t.Run("with detail", func(t *ldtest.T) { executesBeforeEvaluationStageDetail(t, true) })

	t.RequireCapability(servicedef.CapabilityMigrations)
	t.Run("for migrations", executesBeforeEvaluationStageMigration)
}

func executesAfterEvaluationStage(t *ldtest.T) {
	t.Run("without detail", func(t *ldtest.T) { executesAfterEvaluationStageDetail(t, false) })
	t.Run("with detail", func(t *ldtest.T) { executesAfterEvaluationStageDetail(t, true) })

	t.RequireCapability(servicedef.CapabilityMigrations)
	t.Run("for migrations", executesAfterEvaluationStageMigration)
}

func beforeEvaluationDataPropagatesToAfter(t *ldtest.T) {
	t.Run("without detail", func(t *ldtest.T) { beforeEvaluationDataPropagatesToAfterDetail(t, false) })
	t.Run("with detail", func(t *ldtest.T) { beforeEvaluationDataPropagatesToAfterDetail(t, true) })
}

type VariationParameters struct {
	name         string
	flagKey      string
	defaultValue ldvalue.Value
	valueType    servicedef.ValueType
	detail       bool
}

func variationTestParams(detail bool) []VariationParameters {
	return []VariationParameters{{
		name:         "for boolean variation",
		flagKey:      "bool-flag",
		defaultValue: ldvalue.Bool(false),
		valueType:    servicedef.ValueTypeBool,
		detail:       detail,
	},
		{
			name:         "for string variation",
			flagKey:      "string-flag",
			defaultValue: ldvalue.String("default"),
			valueType:    servicedef.ValueTypeString,
			detail:       detail,
		},
		{
			name:         "for double variation",
			flagKey:      "number-flag",
			defaultValue: ldvalue.Float64(3.14),
			valueType:    servicedef.ValueTypeDouble,
			detail:       detail,
		},
		{
			name:         "for int variation",
			flagKey:      "number-flag",
			defaultValue: ldvalue.Int(314159265),
			valueType:    servicedef.ValueTypeInt,
			detail:       detail,
		},
		{
			name:         "for json variation",
			flagKey:      "json-flag",
			defaultValue: ldvalue.ObjectBuild().Build(),
			valueType:    servicedef.ValueTypeAny,
			detail:       detail,
		},
	}
}

func executesBeforeEvaluationStageDetail(t *ldtest.T, detail bool) {
	testParams := variationTestParams(detail)

	hookName := "executesBeforeEvaluationStage"

	context := ldcontext.New("user-key")
	flagContext := o.Some(context)
	configurers := []SDKConfigurer{}

	if t.Capabilities().Has(servicedef.CapabilityClientSide) {
		configurers = append(configurers, WithClientSideInitialContext(context))
		flagContext = o.None[ldcontext.Context]()
	}

	client, hooks := createClientForHooks(t, []string{hookName}, nil, configurers...)
	defer hooks.Close()

	for _, testParam := range testParams {
		t.Run(testParam.name, func(t *ldtest.T) {
			client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
				FlagKey:      testParam.flagKey,
				Context:      flagContext,
				ValueType:    testParam.valueType,
				DefaultValue: testParam.defaultValue,
			})

			hooks.ExpectCall(t, hookName, func(payload servicedef.HookExecutionPayload) bool {
				if payload.Stage.Value() == servicedef.BeforeEvaluation {
					hookContext := payload.EvaluationSeriesContext.Value()
					assert.Equal(t, testParam.flagKey, hookContext.FlagKey)
					assert.Equal(t, context, hookContext.Context)
					assert.Equal(t, testParam.defaultValue, hookContext.DefaultValue)
					return true
				}
				return false
			})
		})
	}
}

func executesBeforeEvaluationStageMigration(t *ldtest.T) {
	hookName := "executesBeforeEvaluationStageMigration"
	client, hooks := createClientForHooks(t, []string{hookName}, nil)
	defer hooks.Close()

	flagKey := "migration-flag"
	params := servicedef.MigrationVariationParams{
		Key:          flagKey,
		Context:      ldcontext.New("user-key"),
		DefaultStage: ldmigration.Off,
	}
	client.MigrationVariation(t, params)

	hooks.ExpectCall(t, hookName, func(payload servicedef.HookExecutionPayload) bool {
		if payload.Stage.Value() == servicedef.BeforeEvaluation {
			hookContext := payload.EvaluationSeriesContext.Value()
			assert.Equal(t, flagKey, hookContext.FlagKey)
			assert.Equal(t, ldcontext.New("user-key"), hookContext.Context)
			assert.Equal(t, ldvalue.String(string(ldmigration.Off)), hookContext.DefaultValue)
			return true
		}
		return false
	})
}

func executesAfterEvaluationStageDetail(t *ldtest.T, detail bool) {
	testParams := variationTestParams(detail)

	hookName := "executesAfterEvaluationStage"

	context := ldcontext.New("user-key")
	flagContext := o.Some(context)
	configurers := []SDKConfigurer{}

	if t.Capabilities().Has(servicedef.CapabilityClientSide) {
		configurers = append(configurers, WithClientSideInitialContext(context))
		flagContext = o.None[ldcontext.Context]()
	}

	client, hooks := createClientForHooks(t, []string{hookName}, nil, configurers...)
	defer hooks.Close()

	for _, testParam := range testParams {
		t.Run(testParam.name, func(t *ldtest.T) {
			result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
				FlagKey:      testParam.flagKey,
				Context:      flagContext,
				ValueType:    testParam.valueType,
				DefaultValue: testParam.defaultValue,
				Detail:       detail,
			})

			hooks.ExpectCall(t, hookName, func(payload servicedef.HookExecutionPayload) bool {
				if payload.Stage.Value() == servicedef.AfterEvaluation {
					hookContext := payload.EvaluationSeriesContext.Value()
					assert.Equal(t, testParam.flagKey, hookContext.FlagKey)
					assert.Equal(t, context, hookContext.Context)
					assert.Equal(t, testParam.defaultValue, hookContext.DefaultValue)
					evaluationDetail := payload.EvaluationDetail.Value()
					assert.Equal(t, evaluationDetail.Value, result.Value)
					if detail {
						assert.Equal(t, result.VariationIndex, evaluationDetail.VariationIndex)
						assert.Equal(t, result.Reason, evaluationDetail.Reason)
					}
					return true
				}
				return false
			})
		})
	}
}

func executesAfterEvaluationStageMigration(t *ldtest.T) {
	hookName := "executesBeforeEvaluationStageMigration"
	client, hooks := createClientForHooks(t, []string{hookName}, nil)
	defer hooks.Close()

	flagKey := "migration-flag"
	params := servicedef.MigrationVariationParams{
		Key:          flagKey,
		Context:      ldcontext.New("user-key"),
		DefaultStage: ldmigration.Off,
	}
	result := client.MigrationVariation(t, params)

	hooks.ExpectCall(t, hookName, func(payload servicedef.HookExecutionPayload) bool {
		if payload.Stage.Value() == servicedef.AfterEvaluation {
			hookContext := payload.EvaluationSeriesContext.Value()
			assert.Equal(t, flagKey, hookContext.FlagKey)
			assert.Equal(t, ldcontext.New("user-key"), hookContext.Context)
			assert.Equal(t, ldvalue.String(string(ldmigration.Off)), hookContext.DefaultValue)
			evaluationDetail := payload.EvaluationDetail.Value()
			assert.Equal(t, ldvalue.String(result.Result), evaluationDetail.Value)
			return true
		}
		return false
	})
}

func beforeEvaluationDataPropagatesToAfterDetail(t *ldtest.T, detail bool) {
	testParams := variationTestParams(detail)

	hookName := "beforeEvaluationDataPropagatesToAfterDetail"
	hookData := make(map[servicedef.HookStage]servicedef.SDKConfigEvaluationHookData)
	hookData[servicedef.BeforeEvaluation] = make(servicedef.SDKConfigEvaluationHookData)
	hookData[servicedef.BeforeEvaluation]["someData"] = ldvalue.String("the hookData")

	context := ldcontext.New("user-key")
	flagContext := o.Some(context)
	configurers := []SDKConfigurer{}

	if t.Capabilities().Has(servicedef.CapabilityClientSide) {
		configurers = append(configurers, WithClientSideInitialContext(context))
		flagContext = o.None[ldcontext.Context]()
	}

	client, hooks := createClientForHooks(t, []string{hookName}, hookData, configurers...)
	defer hooks.Close()

	for _, testParam := range testParams {
		t.Run(testParam.name, func(t *ldtest.T) {
			client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
				FlagKey:      testParam.flagKey,
				Context:      flagContext,
				ValueType:    testParam.valueType,
				DefaultValue: testParam.defaultValue,
				Detail:       detail,
			})

			hooks.ExpectCall(t, hookName, func(payload servicedef.HookExecutionPayload) bool {
				if payload.Stage.Value() == servicedef.AfterEvaluation {
					hookData := payload.EvaluationSeriesData.Value()
					assert.Equal(t, ldvalue.String("the hookData"), hookData["someData"])
					assert.Len(t, hookData, 1)
					return true
				}
				return false
			})
		})
	}
}

func beforeEvaluationDataPropagatesToAfterMigration(t *ldtest.T) {
	hookName := "beforeEvaluationDataPropagatesToAfterDetail"
	hookData := make(map[servicedef.HookStage]servicedef.SDKConfigEvaluationHookData)
	hookData[servicedef.BeforeEvaluation] = make(servicedef.SDKConfigEvaluationHookData)
	hookData[servicedef.BeforeEvaluation]["someData"] = ldvalue.String("the hookData")

	client, hooks := createClientForHooks(t, []string{hookName}, hookData)
	defer hooks.Close()

	flagKey := "migration-flag"
	params := servicedef.MigrationVariationParams{
		Key:          flagKey,
		Context:      ldcontext.New("user-key"),
		DefaultStage: ldmigration.Off,
	}
	client.MigrationVariation(t, params)

	hooks.ExpectCall(t, hookName, func(payload servicedef.HookExecutionPayload) bool {
		if payload.Stage.Value() == servicedef.AfterEvaluation {
			hookData := payload.EvaluationSeriesData.Value()
			assert.Equal(t, ldvalue.String("the hookData"), hookData["someData"])
			assert.Len(t, hookData, 1)
			return true
		}
		return false
	})
}

// This test is meant to check Requirement HOOKS:1.3.7:
// The client MUST handle exceptions which are thrown (or errors returned, if idiomatic for the language)
// during the execution of a stage or handler allowing operations to complete unaffected.
func errorInBeforeStageDoesNotAffectAfterStage(t *ldtest.T) {
	const numHooks = 3

	// We're configuring the beforeEvaluation stage with some data, but we don't expect
	// to see it propagated into afterEvaluation since we're also configuring beforeEvaluation
	// to throw an exception (or return an error, whatever is appropriate for the language.)
	hookData := map[servicedef.HookStage]servicedef.SDKConfigEvaluationHookData{
		servicedef.BeforeEvaluation: map[string]ldvalue.Value{"this_value": ldvalue.String("should_not_be_received")},
	}

	var names []string
	for i := 0; i < numHooks; i++ {
		names = append(names, "fallibleHook-"+strconv.Itoa(i))
	}

	context := ldcontext.New("user-key")
	flagContext := o.Some(context)
	configurers := []SDKConfigurer{}

	if t.Capabilities().Has(servicedef.CapabilityClientSide) {
		configurers = append(configurers, WithClientSideInitialContext(context))
		flagContext = o.None[ldcontext.Context]()
	}

	client, hooks := createClientForHooksWithErrors(t, names, hookData, map[servicedef.HookStage]o.Maybe[string]{
		servicedef.BeforeEvaluation: o.Some("something is rotten in the state of Denmark!"),
	}, configurers...)

	defer hooks.Close()

	flagKey := "bool-flag"
	client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey:      flagKey,
		Context:      flagContext,
		ValueType:    servicedef.ValueTypeBool,
		DefaultValue: ldvalue.Bool(false),
	})

	calls := hooks.ExpectAtLeastOneCallForEachHook(t, names)

	for _, call := range calls {
		assert.Equal(t, servicedef.AfterEvaluation, call.Stage.Value(), "HOOKS:1.3.7: beforeEvaluation "+
			"should not have caused a POST to the test harness; ensure exception is thrown/error "+
			"returned in this stage")

		assert.Equal(t, 0, len(call.EvaluationSeriesData.Value()), "HOOKS:1.3.7.1: Since "+
			"beforeEvaluation should have failed, the data passed to afterEvaluation should be empty")
	}
}

func createClientForHooks(t *ldtest.T, instances []string,
	hookData map[servicedef.HookStage]servicedef.SDKConfigEvaluationHookData,
	configurers ...SDKConfigurer) (*SDKClient, *Hooks) {
	return createClientForHooksWithErrors(t, instances, hookData, nil, configurers...)
}

func createClientForHooksWithErrors(t *ldtest.T, instances []string,
	hookData map[servicedef.HookStage]servicedef.SDKConfigEvaluationHookData,
	hookErrors map[servicedef.HookStage]o.Maybe[string], configurers ...SDKConfigurer) (*SDKClient, *Hooks) {
	boolFlag := ldbuilders.NewFlagBuilder("bool-flag").
		Variations(ldvalue.Bool(false), ldvalue.Bool(true)).
		FallthroughVariation(1).On(true).Build()

	numberFlag := ldbuilders.NewFlagBuilder("number-flag").
		Variations(ldvalue.Int(0), ldvalue.Int(42)).
		OffVariation(1).On(false).Build()

	stringFlag := ldbuilders.NewFlagBuilder("string-flag").
		Variations(ldvalue.String("string-off"), ldvalue.String("string-on")).
		FallthroughVariation(1).On(true).Build()

	jsonFlag := ldbuilders.NewFlagBuilder("json-flag").
		Variations(ldvalue.ObjectBuild().Set("value", ldvalue.Bool(false)).Build(),
			ldvalue.ObjectBuild().Set("value", ldvalue.Bool(true)).Build()).
		FallthroughVariation(1).On(true).Build()
	migrationFlag := ldbuilders.NewFlagBuilder("migration-flag").
		On(true).
		Variations(data.MakeStandardMigrationStages()...).
		FallthroughVariation(1).
		Build()

	flags := []ldmodel.FeatureFlag{
		boolFlag,
		numberFlag,
		stringFlag,
		jsonFlag,
		migrationFlag,
	}

	var dataSource *SDKDataSource
	if t.Capabilities().Has(servicedef.CapabilityClientSide) {
		dataBuilder := mockld.NewClientSDKDataBuilder()
		for _, flag := range flags {
			dataBuilder.Flag(flag.Key, mockld.ClientSDKFlag{Value: flag.Variations[1]})
		}
		dataSource = NewSDKDataSource(t, dataBuilder.Build())
	} else {
		dataBuilder := mockld.NewServerSDKDataBuilder()
		dataBuilder.Flag(flags...)
		dataSource = NewSDKDataSource(t, dataBuilder.Build())
	}

	hooks := NewHooks(requireContext(t).harness, t.DebugLogger(), instances, hookData, hookErrors)
	events := NewSDKEventSink(t)

	configurers = append(configurers, dataSource, hooks, events)
	client := NewSDKClient(t, configurers...)
	return client, hooks
}
