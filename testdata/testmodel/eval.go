package testmodel

import (
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
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
	User      lduser.User          `json:"user"`
	ValueType servicedef.ValueType `json:"valueType"`
	Default   ldvalue.Value        `json:"default"`
	Expect    ValueDetail          `json:"expect"`
}

type ValueDetail struct {
	Value          ldvalue.Value             `json:"value"`
	VariationIndex o.Maybe[int]              `json:"variationIndex"`
	Reason         ldreason.EvaluationReason `json:"reason"`
}
