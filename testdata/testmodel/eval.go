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

type EvalTestSuite[SDKDataType any] struct {
	Name                 string               `json:"name"`
	RequireCapability    string               `json:"requireCapability"`
	SkipEvaluateAllFlags bool                 `json:"skipEvaluateAllFlags"`
	SDKData              SDKDataType          `json:"sdkData"`
	User                 o.Maybe[lduser.User] `json:"user"` // used only for client-side tests
	Evaluations          []EvalTest           `json:"evaluations"`
}

type ServerSideEvalTestSuite EvalTestSuite[mockld.ServerSDKData]

type ClientSideEvalTestSuite EvalTestSuite[mockld.ClientSDKData]

type EvalTest struct {
	Name      string               `json:"name"`
	FlagKey   string               `json:"flagKey"`
	User      o.Maybe[lduser.User] `json:"user"` // used only for server-side tests
	ValueType servicedef.ValueType `json:"valueType"`
	Default   ldvalue.Value        `json:"default"`
	Expect    ValueDetail          `json:"expect"`
}

type ValueDetail struct {
	Value          ldvalue.Value             `json:"value"`
	VariationIndex o.Maybe[int]              `json:"variationIndex"`
	Reason         ldreason.EvaluationReason `json:"reason"`
}
