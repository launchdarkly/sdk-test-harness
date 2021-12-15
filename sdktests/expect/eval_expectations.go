package expect

import (
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"

	"github.com/stretchr/testify/assert"
)

var Eval EvalExpectationFactory //nolint:gochecknoglobals

type EvalExpectationFactory struct{}

type EvalExpectation struct {
	base helpers.Expectation
}

func (x EvalExpectation) For(t assert.TestingT, er servicedef.EvaluateFlagResponse) bool {
	return x.base.For(t, er)
}

func (x EvalExpectation) And(other EvalExpectation) EvalExpectation {
	return EvalExpectation{base: x.base.And(other.base)}
}

func (f EvalExpectationFactory) New(
	description string,
	fn func(t assert.TestingT, er servicedef.EvaluateFlagResponse) bool,
) EvalExpectation {
	base := helpers.NewExpectation(
		description,
		func(value interface{}) string {
			r := value.(servicedef.EvaluateFlagResponse)
			if r.VariationIndex == nil && r.Reason == nil {
				return fmt.Sprintf(`{value: %s}`, r.Value.JSONString())
			} else {
				reasonDesc := "[none]"
				if r.Reason != nil {
					reasonDesc = helpers.AsJSONString(*r.Reason)
				}
				return fmt.Sprintf(`{value: %s, variationIndex: %s, reason: %s}`, r.Value.JSONString(),
					ldvalue.NewOptionalIntFromPointer(r.VariationIndex), reasonDesc)
			}
		},
		func(t assert.TestingT, value interface{}) bool {
			return fn == nil || fn(t, value.(servicedef.EvaluateFlagResponse))
		},
	)
	return EvalExpectation{base}
}

func (f EvalExpectationFactory) ValueEquals(value ldvalue.Value) EvalExpectation {
	return f.New(fmt.Sprintf("result value is %s", value.JSONString()),
		func(t assert.TestingT, er servicedef.EvaluateFlagResponse) bool {
			return helpers.AssertJSONEqual(t, value.JSONString(), er.Value.JSONString())
		})
}

func (f EvalExpectationFactory) VariationEquals(variation ldvalue.OptionalInt) EvalExpectation {
	return f.New(fmt.Sprintf("result variation index is %s", variation),
		func(t assert.TestingT, er servicedef.EvaluateFlagResponse) bool {
			return assert.Equal(t, variation, ldvalue.NewOptionalIntFromPointer(er.VariationIndex))
		})
}

func (f EvalExpectationFactory) ReasonEquals(reason ldreason.EvaluationReason) EvalExpectation {
	return f.New(fmt.Sprintf("result reason is %s", helpers.AsJSONString(reason)),
		func(t assert.TestingT, er servicedef.EvaluateFlagResponse) bool {
			return helpers.AssertJSONEqual(t, helpers.AsJSONString(reason), helpers.AsJSONString(er.Reason))
		})
}
