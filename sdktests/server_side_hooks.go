package sdktests

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldmigration"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
	"github.com/stretchr/testify/assert"
	"time"
)

func doServerSideHooksTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityEvaluationHooks)
	t.Run("executes beforeEvaluation stage", executesBeforeEvaluationStage)
	t.Run("executes afterEvaluation stage", executesAfterEvaluationStage)
	t.Run("data propagates from before to after", beforeEvaluationDataPropagatesToAfter)
	t.Run("data propagates from before to after for migrations", beforeEvaluationDataPropagatesToAfterMigration)
}

func executesBeforeEvaluationStage(t *ldtest.T) {
	t.Run("without detail", func(t *ldtest.T) { executesBeforeEvaluationStageDetail(t, false) })
	t.Run("with detail", func(t *ldtest.T) { executesBeforeEvaluationStageDetail(t, true) })
	t.Run("for migrations", executesBeforeEvaluationStageMigration)
}

func executesAfterEvaluationStage(t *ldtest.T) {
	t.Run("without detail", func(t *ldtest.T) { executesAfterEvaluationStageDetail(t, false) })
	t.Run("with detail", func(t *ldtest.T) { executesAfterEvaluationStageDetail(t, true) })
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
			defaultValue: ldvalue.Int(0xDEADBEEF),
			valueType:    servicedef.ValueTypeInt,
			detail:       detail,
		},
		{
			name:         "for json variation",
			flagKey:      "json-flag",
			defaultValue: ldvalue.ObjectBuild().Build(),
			valueType:    servicedef.ValueTypeInt,
			detail:       detail,
		},
	}
}

func executesBeforeEvaluationStageDetail(t *ldtest.T, detail bool) {
	testParams := variationTestParams(detail)

	hookName := "executesBeforeEvaluationStage"
	client, hooks := createClientForHooks(t, []string{hookName}, nil)
	defer hooks.Close()

	for _, testParam := range testParams {
		t.Run(testParam.name, func(t *ldtest.T) {
			client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
				FlagKey:      testParam.flagKey,
				Context:      o.Some(ldcontext.New("user-key")),
				ValueType:    testParam.valueType,
				DefaultValue: testParam.defaultValue,
			})

			hooks.ExpectCall(t, hookName, 1*time.Second, func(payload servicedef.HookExecutionPayload) bool {
				if payload.Stage.Value() == servicedef.BeforeEvaluation {
					hookContext := payload.EvaluationHookContext.Value()
					assert.Equal(t, testParam.flagKey, hookContext.FlagKey)
					assert.Equal(t, ldcontext.New("user-key"), hookContext.Context)
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

	hooks.ExpectCall(t, hookName, 1*time.Second, func(payload servicedef.HookExecutionPayload) bool {
		if payload.Stage.Value() == servicedef.BeforeEvaluation {
			hookContext := payload.EvaluationHookContext.Value()
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
	client, hooks := createClientForHooks(t, []string{hookName}, nil)
	defer hooks.Close()

	for _, testParam := range testParams {
		t.Run(testParam.name, func(t *ldtest.T) {
			result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
				FlagKey:      testParam.flagKey,
				Context:      o.Some(ldcontext.New("user-key")),
				ValueType:    testParam.valueType,
				DefaultValue: testParam.defaultValue,
				Detail:       detail,
			})

			hooks.ExpectCall(t, hookName, 1*time.Second, func(payload servicedef.HookExecutionPayload) bool {
				if payload.Stage.Value() == servicedef.AfterEvaluation {
					hookContext := payload.EvaluationHookContext.Value()
					assert.Equal(t, testParam.flagKey, hookContext.FlagKey)
					assert.Equal(t, ldcontext.New("user-key"), hookContext.Context)
					assert.Equal(t, testParam.defaultValue, hookContext.DefaultValue)
					evaluationDetail := payload.EvaluationDetail.Value()
					assert.Equal(t, result.Value, evaluationDetail.Value)
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

	hooks.ExpectCall(t, hookName, 1*time.Second, func(payload servicedef.HookExecutionPayload) bool {
		if payload.Stage.Value() == servicedef.AfterEvaluation {
			hookContext := payload.EvaluationHookContext.Value()
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
	hookData := make(map[servicedef.HookStage]map[string]ldvalue.Value)
	hookData[servicedef.BeforeEvaluation] = make(map[string]ldvalue.Value)
	hookData[servicedef.BeforeEvaluation]["someData"] = ldvalue.String("the hookData")

	client, hooks := createClientForHooks(t, []string{hookName}, hookData)
	defer hooks.Close()

	for _, testParam := range testParams {
		t.Run(testParam.name, func(t *ldtest.T) {
			client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
				FlagKey:      testParam.flagKey,
				Context:      o.Some(ldcontext.New("user-key")),
				ValueType:    testParam.valueType,
				DefaultValue: testParam.defaultValue,
				Detail:       detail,
			})

			hooks.ExpectCall(t, hookName, 1*time.Second, func(payload servicedef.HookExecutionPayload) bool {
				if payload.Stage.Value() == servicedef.AfterEvaluation {
					hookData := payload.EvaluationHookData.Value()
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
	hookData := make(map[servicedef.HookStage]map[string]ldvalue.Value)
	hookData[servicedef.BeforeEvaluation] = make(map[string]ldvalue.Value)
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

	hooks.ExpectCall(t, hookName, 1*time.Second, func(payload servicedef.HookExecutionPayload) bool {
		if payload.Stage.Value() == servicedef.AfterEvaluation {
			hookData := payload.EvaluationHookData.Value()
			assert.Equal(t, ldvalue.String("the hookData"), hookData["someData"])
			assert.Len(t, hookData, 1)
			return true
		}
		return false
	})
}

func createClientForHooks(t *ldtest.T, instances []string, hookData map[servicedef.HookStage]map[string]ldvalue.Value) (*SDKClient, *Hooks) {
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

	dataBuilder := mockld.NewServerSDKDataBuilder()
	dataBuilder.Flag(boolFlag, numberFlag, stringFlag, jsonFlag, migrationFlag)

	hooks := NewHooks(requireContext(t).harness, t.DebugLogger(), instances, hookData)

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSource, hooks, events)
	return client, hooks
}
