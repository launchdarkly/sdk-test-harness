package testmodel

import (
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

const DefaultForAllTypes servicedef.ValueType = "allDefaults"

// TestSpecReference links a test or test suite to a formal specification requirement.
type TestSpecReference struct {
	Spec    string `json:"spec"`
	Number  string `json:"number"`
	Summary string `json:"summary"`
}

type EvalTestSuite[SDKDataType any] struct {
	Name                 string                     `json:"name"`
	RequireCapability    string                     `json:"requireCapability"`
	SkipEvaluateAllFlags bool                       `json:"skipEvaluateAllFlags"`
	Specifications       []TestSpecReference        `json:"specifications,omitempty"`
	SDKData              SDKDataType                `json:"sdkData"`
	Context              o.Maybe[ldcontext.Context] `json:"context"` // used only for client-side tests
	Evaluations          []EvalTest                 `json:"evaluations"`
}

func (s EvalTestSuite[SDKDataType]) GetName() string { return s.Name }

type EvalTest struct {
	Name      string                     `json:"name"`
	FlagKey   string                     `json:"flagKey"`
	Context   o.Maybe[ldcontext.Context] `json:"context"` // used only for server-side tests
	ValueType servicedef.ValueType       `json:"valueType"`
	Default   ldvalue.Value              `json:"default"`
	Expect    ValueDetail                `json:"expect"`
}

type ValueDetail struct {
	Value          ldvalue.Value             `json:"value"`
	VariationIndex o.Maybe[int]              `json:"variationIndex"`
	Reason         ldreason.EvaluationReason `json:"reason"`
}
