package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/framework/matchers"
	"github.com/launchdarkly/sdk-test-harness/servicedef"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

// The functions in this file are for convenient use of the framework/matchers API with
// complex types. For more information, see matchers.MatcherTransform.

func EvalResponseValue() matchers.MatcherTransform {
	return matchers.Transform(
		"result value",
		func(value interface{}) interface{} {
			r := value.(servicedef.EvaluateFlagResponse)
			return r.Value
		}).
		EnsureInputValueType(servicedef.EvaluateFlagResponse{}).
		// default String() formatting of EvaluateFlagResponse isn't desirable
		WithInputValueDescription(matchers.JSONDescription)
}

func EvalResponseVariation() matchers.MatcherTransform {
	return matchers.Transform(
		"result variation index",
		func(value interface{}) interface{} {
			r := value.(servicedef.EvaluateFlagResponse)
			return ldvalue.NewOptionalIntFromPointer(r.VariationIndex)
		}).
		EnsureInputValueType(servicedef.EvaluateFlagResponse{}).
		// default String() formatting of EvaluateFlagResponse isn't desirable
		WithInputValueDescription(matchers.JSONDescription)
}

func EvalResponseReason() matchers.MatcherTransform {
	return matchers.Transform(
		"result reason",
		func(value interface{}) interface{} {
			r := value.(servicedef.EvaluateFlagResponse)
			if r.Reason == nil {
				return nil
			}
			return *r.Reason
		}).
		EnsureInputValueType(servicedef.EvaluateFlagResponse{}).
		// default String() formatting of EvaluateFlagResponse isn't desirable
		WithInputValueDescription(matchers.JSONDescription).
		// defaultString() formatting of EvaluationReason isn't desirable
		WithOutputValueDescription(matchers.JSONDescription)
}
