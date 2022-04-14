package testmodel

import (
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

const DefaultForAllTypes servicedef.ValueType = "allDefaults"

type EvalTestSuite struct {
	Name                 string               `json:"name"`
	RequireCapability    string               `json:"requireCapability"`
	SkipEvaluateAllFlags bool                 `json:"skipEvaluateAllFlags"`
	SDKData              mockld.ServerSDKData `json:"sdkData"`
	Evaluations          []EvalTest           `json:"evaluations"`
}

type EvalTest struct {
	Name      string               `json:"name"`
	FlagKey   string               `json:"flagKey"`
	Context   ldcontext.Context    `json:"context"`
	ValueType servicedef.ValueType `json:"valueType"`
	Default   ldvalue.Value        `json:"default"`
	Expect    ValueDetail          `json:"expect"`
}

type ValueDetail struct {
	Value          ldvalue.Value             `json:"value"`
	VariationIndex o.Maybe[int]              `json:"variationIndex"`
	Reason         ldreason.EvaluationReason `json:"reason"`
}