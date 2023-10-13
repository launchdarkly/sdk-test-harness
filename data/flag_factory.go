package data

import (
	"fmt"
	"time"

	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
)

// FlagFactory is a test data generator that produces ldmodel.FeatureFlag instances.
type FlagFactory struct {
	factory        *MemoizingFactory[servicedef.ValueType, ldmodel.FeatureFlag]
	keyPrefix      string
	builderActions []func(*ldbuilders.FlagBuilder)
	valueFactory   ValueFactoryBySDKValueType
	counter        int
}

// NewFlagFactory creates a FlagFactory with the specified configuration.
//
// The valueFactory parameter provides the value that each flag will return for evaluations.
// The builderActions, if any, will be run each time a flag is created. Each flag will have
// a unique key beginning with the specified prefix.
func NewFlagFactory(
	keyPrefix string,
	valueFactory ValueFactoryBySDKValueType,
	builderActions ...func(*ldbuilders.FlagBuilder),
) *FlagFactory {
	f := &FlagFactory{
		keyPrefix:      keyPrefix,
		valueFactory:   valueFactory,
		builderActions: builderActions,
	}
	f.factory = NewMemoizingFlagFactory(0,
		func(valueType servicedef.ValueType) ldmodel.FeatureFlag {
			f.counter++
			flagKey := fmt.Sprintf("%s.%d", f.keyPrefix, f.counter)
			if valueType == "" {
				valueType = servicedef.ValueTypeString
			} else {
				flagKey += "." + string(valueType)
			}
			builder := ldbuilders.NewFlagBuilder(flagKey)
			builder.Variations(f.valueFactory(valueType))
			for _, ba := range f.builderActions {
				ba(builder)
			}
			flag := builder.Build()
			return flag
		})
	return f
}

// MakeFlag creates a new flag configuration. Use this when the value type is not significant to the test;
// it will default to using string variations, since those are more easily readable in test output.
func (f *FlagFactory) MakeFlag() ldmodel.FeatureFlag {
	return f.MakeFlagForValueType("")
}

// MakeFlagForValueType creates a new flag configuration. The flag variations will be of the specified type.
func (f *FlagFactory) MakeFlagForValueType(valueType servicedef.ValueType) ldmodel.FeatureFlag {
	return f.factory.Create(valueType)
}

// FlagShouldAlwaysHaveDebuggingEnabled is a convenience function for configuring a flag to have debugging
// enabled (by setting DebugEventsUntilDate to a far future time).
func FlagShouldAlwaysHaveDebuggingEnabled(builder *ldbuilders.FlagBuilder) {
	builder.DebugEventsUntilDate(ldtime.UnixMillisNow() + 10000000)
}

// FlagShouldHaveDebuggingEnabledUntil is a convenience function for configuring a flag to have debugging
// enabled until the specified time.
func FlagShouldHaveDebuggingEnabledUntil(t time.Time) func(*ldbuilders.FlagBuilder) {
	return func(builder *ldbuilders.FlagBuilder) {
		builder.DebugEventsUntilDate(ldtime.UnixMillisFromTime(t))
	}
}

// FlagShouldHaveFullEventTracking is a convenience function for configuring a flag to have full
// event tracking enabled (by setting TrackEvents to true).
func FlagShouldHaveFullEventTracking(builder *ldbuilders.FlagBuilder) {
	builder.TrackEvents(true)
}

// FlagShouldProduceThisEvalReason is a convenience function for configuring a flag to produce a
// specific evaluation reason for all evaluations. If specific contexts should be matched, pass
// them in matchContexts-- however, this implementation only works if they are single-kind contexts
// that are all of the same kind.
func FlagShouldProduceThisEvalReason(
	reason ldreason.EvaluationReason,
	matchContexts ...ldcontext.Context,
) func(*ldbuilders.FlagBuilder) {
	getContextKindAndKeys := func() (ldcontext.Kind, []string) {
		var keys []string
		kind := ldcontext.DefaultKind
		for _, context := range matchContexts {
			keys = append(keys, context.Key())
			kind = context.Kind()
		}
		return kind, keys
	}
	return func(builder *ldbuilders.FlagBuilder) {
		switch reason.GetKind() {
		case ldreason.EvalReasonOff:
			builder.On(false).OffVariation(0)
		case ldreason.EvalReasonFallthrough:
			builder.On(true).FallthroughVariation(0)
		case ldreason.EvalReasonTargetMatch:
			builder.On(true)
			kind, keys := getContextKindAndKeys()
			if kind == ldcontext.DefaultKind {
				builder.AddTarget(0, keys...)
			} else {
				builder.AddContextTarget(kind, 0, keys...)
			}
		case ldreason.EvalReasonRuleMatch:
			builder.On(true).FallthroughVariation(0)
			kind, keys := getContextKindAndKeys()
			for i := 0; i < reason.GetRuleIndex(); i++ { // add some never-matching rules to get to the desired index
				builder.AddRule(ldbuilders.NewRuleBuilder().Clauses(ldbuilders.Clause("key", "in", ldvalue.Null())))
			}
			rule := ldbuilders.NewRuleBuilder().ID(reason.GetRuleID()).Variation(0)
			if len(keys) != 0 {
				values := make([]ldvalue.Value, 0, len(keys))
				for _, key := range keys {
					values = append(values, ldvalue.String(key))
				}
				rule.Clauses(ldbuilders.ClauseWithKind(kind, "key", "in", values...))
			} else {
				rule.Clauses(ldbuilders.Negate(ldbuilders.Clause("kind", "in", ldvalue.String(""))))
			}
			builder.AddRule(rule)
		case ldreason.EvalReasonError:
			builder.On(false).OffVariation(-1)
		}
	}
}

// ReuseFlagForValueType is the same as MakeFlagForValueType except that if MakeFlagForValueType
// has already been called for the same type, it will return the same flag and not create a new one.
func (f *FlagFactory) ReuseFlagForValueType(valueType servicedef.ValueType) ldmodel.FeatureFlag {
	return f.factory.GetOrCreate(valueType)
}

// ClientSideFlagFactory is a test data generator that produces mockld.ClientSDKFlagWithKey instances.
type ClientSideFlagFactory struct {
	factory        *MemoizingFactory[servicedef.ValueType, mockld.ClientSDKFlagWithKey]
	keyPrefix      string
	valueFactory   ValueFactoryBySDKValueType
	builderActions []func(*mockld.ClientSDKFlagWithKey)
	nextVariation  int
}

// NewClientSideFlagFactory creates a ClientSideFlagFactory with the specified configuration.
//
// The valueFactory parameter provides the value that each flag will return for evaluations.
// The builderActions, if any, will be run each time a flag is created. Each flag will have
// a unique key beginning with the specified prefix.
func NewClientSideFlagFactory(
	keyPrefix string,
	valueFactory ValueFactoryBySDKValueType,
	builderActions ...func(*mockld.ClientSDKFlagWithKey),
) *ClientSideFlagFactory {
	f := &ClientSideFlagFactory{
		keyPrefix:      keyPrefix,
		valueFactory:   valueFactory,
		builderActions: builderActions,
	}
	f.factory = NewMemoizingClientSideFlagFactory(0,
		func(valueType servicedef.ValueType) mockld.ClientSDKFlagWithKey {
			ret := mockld.ClientSDKFlagWithKey{
				Key: fmt.Sprintf("%s.%s", f.keyPrefix, valueType),
				ClientSDKFlag: mockld.ClientSDKFlag{
					Value:     f.valueFactory(valueType),
					Variation: o.Some(f.nextVariation),
				},
			}
			f.nextVariation = (f.nextVariation + 1) % 5 // arbitrary number of variations just so data isn't uniform
			for _, ba := range f.builderActions {
				ba(&ret)
			}
			return ret
		})
	return f
}

// MakeFlagForValueType creates a new flag configuration. The flag variation will be of the specified type.
func (f *ClientSideFlagFactory) MakeFlagForValueType(valueType servicedef.ValueType) mockld.ClientSDKFlagWithKey {
	return f.factory.Create(valueType)
}

// ReuseFlagForValueType is the same as MakeFlagForValueType except that if MakeFlagForValueType
// has already been called for the same type, it will return the same flag and not create a new one.
func (f *ClientSideFlagFactory) ReuseFlagForValueType(valueType servicedef.ValueType) mockld.ClientSDKFlagWithKey {
	return f.factory.GetOrCreate(valueType)
}

// ClientSideFlagShouldHaveEvalReason is a convenience function for configuring a client-side flag
// to have an evaluation reason.
func ClientSideFlagShouldHaveEvalReason(reason ldreason.EvaluationReason) func(*mockld.ClientSDKFlagWithKey) {
	return func(f *mockld.ClientSDKFlagWithKey) { f.Reason = o.Some(reason) }
}

// ClientSideFlagShouldHaveFullEventTracking is a convenience function for configuring a client-side
// flag to have full event tracking enabled (by setting TrackEvents to true).
func ClientSideFlagShouldHaveFullEventTracking(f *mockld.ClientSDKFlagWithKey) {
	f.TrackEvents = true
}

// ClientSideFlagShouldHaveDebuggingEnabledUntil is a convenience function for configuring a client-side flag to have
// debugging enabled until the specified time.
func ClientSideFlagShouldHaveDebuggingEnabledUntil(t time.Time) func(*mockld.ClientSDKFlagWithKey) {
	return func(f *mockld.ClientSDKFlagWithKey) { f.DebugEventsUntilDate = o.Some(ldtime.UnixMillisFromTime(t)) }
}
