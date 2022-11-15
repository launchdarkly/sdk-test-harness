package sdktests

import (
	"fmt"

	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
)

type UserFactory struct {
	prefix         string
	counter        int
	builderActions []func(lduser.UserBuilder)
}

func NewUserFactory(prefix string, builderActions ...func(lduser.UserBuilder)) *UserFactory {
	return &UserFactory{
		prefix:         fmt.Sprintf("%s.%d", prefix, ldtime.UnixMillisNow()),
		builderActions: builderActions,
	}
}

func (f *UserFactory) NextUniqueUser() lduser.User {
	f.counter++
	key := fmt.Sprintf("%s.%d", f.prefix, f.counter)
	builder := lduser.NewUserBuilder(key)
	for _, ba := range f.builderActions {
		ba(builder)
	}
	return builder.Build()
}

func (f *UserFactory) NextUniqueUserMaybeAnonymous(shouldBeAnonymous bool) lduser.User {
	user := f.NextUniqueUser()
	if shouldBeAnonymous {
		return lduser.NewUserBuilderFromUser(user).Anonymous(true).Build()
	}
	return user
}

type GenericFactory[ParamT, ResultT any] interface {
	Get(param ParamT) ResultT
}

type MemoizingFactory[ParamT comparable, ResultT any] struct {
	factoryFn          func(ParamT) ResultT
	transformVersionFn func(ResultT, int) ResultT
	cache              map[ParamT]ResultT
	nextVersion        int
}

type ValueFactory[ParamT any] func(param ParamT) ldvalue.Value

func SingleValueFactory(value ldvalue.Value) ValueFactory[servicedef.ValueType] {
	return func(servicedef.ValueType) ldvalue.Value { return value }
}

func FlagValueByTypeFactory() ValueFactory[servicedef.ValueType] {
	return func(valueType servicedef.ValueType) ldvalue.Value {
		switch valueType {
		case servicedef.ValueTypeBool:
			return ldvalue.Bool(true)
		case servicedef.ValueTypeInt:
			return ldvalue.Int(123)
		case servicedef.ValueTypeDouble:
			return ldvalue.Float64(200.5)
		case servicedef.ValueTypeString:
			return ldvalue.String("abc")
		default:
			return ldvalue.ObjectBuild().Set("a", ldvalue.Bool(true)).Build()
		}
	}
}

func DefaultValueByTypeFactory() ValueFactory[servicedef.ValueType] {
	return func(valueType servicedef.ValueType) ldvalue.Value {
		switch valueType {
		case servicedef.ValueTypeBool:
			return ldvalue.Bool(false)
		case servicedef.ValueTypeInt:
			return ldvalue.Int(1)
		case servicedef.ValueTypeDouble:
			return ldvalue.Float64(0.5)
		case servicedef.ValueTypeString:
			return ldvalue.String("default")
		default:
			return ldvalue.ObjectBuild().Set("default", ldvalue.Bool(true)).Build()
		}
	}
}

func NewMemoizingFlagFactory(
	startingVersion int,
	factoryFn func(servicedef.ValueType) ldmodel.FeatureFlag,
) *MemoizingFactory[servicedef.ValueType, ldmodel.FeatureFlag] {
	f := &MemoizingFactory[servicedef.ValueType, ldmodel.FeatureFlag]{
		factoryFn: factoryFn,
		transformVersionFn: func(f ldmodel.FeatureFlag, v int) ldmodel.FeatureFlag {
			f.Version = v
			return f
		},
		nextVersion: startingVersion,
	}
	return f
}

func NewMemoizingClientSideFlagFactory(
	startingVersion int,
	factoryFn func(servicedef.ValueType) mockld.ClientSDKFlagWithKey,
) *MemoizingFactory[servicedef.ValueType, mockld.ClientSDKFlagWithKey] {
	f := &MemoizingFactory[servicedef.ValueType, mockld.ClientSDKFlagWithKey]{
		factoryFn: factoryFn,
		transformVersionFn: func(f mockld.ClientSDKFlagWithKey, v int) mockld.ClientSDKFlagWithKey {
			f.Version = v
			return f
		},
		nextVersion: startingVersion,
	}
	return f
}

func (f *MemoizingFactory[P, R]) Get(param P) R {
	if item, ok := f.cache[param]; ok {
		return item
	}
	item := f.factoryFn(param)
	version := f.nextVersion
	if version == 0 {
		version++
	}
	f.nextVersion++
	item = f.transformVersionFn(item, version)
	f.nextVersion = version
	if f.cache == nil {
		f.cache = make(map[P]R)
	}
	f.cache[param] = item
	return item
}

type FlagFactoryForValueTypes struct {
	KeyPrefix       string
	BuilderActions  func(*ldbuilders.FlagBuilder)
	ValueFactory    ValueFactory[servicedef.ValueType]
	Reason          ldreason.EvaluationReason
	StartingVersion int
	factory         *MemoizingFactory[servicedef.ValueType, ldmodel.FeatureFlag]
}

func (f *FlagFactoryForValueTypes) ForType(valueType servicedef.ValueType) ldmodel.FeatureFlag {
	if f.factory == nil {
		if f.ValueFactory == nil {
			f.ValueFactory = FlagValueByTypeFactory()
		}
		f.factory = NewMemoizingFlagFactory(f.StartingVersion, func(valueType servicedef.ValueType) ldmodel.FeatureFlag {
			flagKey := fmt.Sprintf("%s.%s", f.KeyPrefix, valueType)
			builder := ldbuilders.NewFlagBuilder(flagKey)
			builder.Variations(f.ValueFactory(valueType))
			switch f.Reason.GetKind() {
			case ldreason.EvalReasonFallthrough:
				builder.On(true).FallthroughVariation(0)
			default:
				builder.On(false).OffVariation(0)
			}
			if f.BuilderActions != nil {
				f.BuilderActions(builder)
			}
			return builder.Build()
		})
	}
	return f.factory.Get(valueType)
}

type ClientSideFlagFactoryForValueTypes struct {
	KeyPrefix       string
	BuilderActions  func(*mockld.ClientSDKFlagWithKey)
	ValueFactory    ValueFactory[servicedef.ValueType]
	Reason          ldreason.EvaluationReason
	StartingVersion int
	factory         *MemoizingFactory[servicedef.ValueType, mockld.ClientSDKFlagWithKey]
	nextVariation   int
}

func (f *ClientSideFlagFactoryForValueTypes) ForType(valueType servicedef.ValueType) mockld.ClientSDKFlagWithKey {
	if f.factory == nil {
		if f.ValueFactory == nil {
			f.ValueFactory = FlagValueByTypeFactory()
		}
		f.factory = NewMemoizingClientSideFlagFactory(f.StartingVersion,
			func(valueType servicedef.ValueType) mockld.ClientSDKFlagWithKey {
				ret := mockld.ClientSDKFlagWithKey{
					Key: fmt.Sprintf("%s.%s", f.KeyPrefix, valueType),
					ClientSDKFlag: mockld.ClientSDKFlag{
						Value:     f.ValueFactory(valueType),
						Variation: o.Some(f.nextVariation),
					},
				}
				f.nextVariation = (f.nextVariation + 1) % 5 // arbitrary number of variations just so data isn't uniform
				if f.Reason.IsDefined() {
					ret.Reason = o.Some(f.Reason)
				}
				if f.BuilderActions != nil {
					f.BuilderActions(&ret)
				}
				return ret
			})
	}
	return f.factory.Get(valueType)
}
