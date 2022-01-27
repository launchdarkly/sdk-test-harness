package sdktests

import (
	"fmt"

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

type FlagFactory interface {
	MakeFlag(param interface{}) ldmodel.FeatureFlag
}

type MemoizingFlagFactory struct {
	factoryFn   func(interface{}) ldmodel.FeatureFlag
	flags       map[interface{}]ldmodel.FeatureFlag
	nextVersion int
}

type ValueFactory func(param interface{}) ldvalue.Value

func SingleValueFactory(value ldvalue.Value) ValueFactory {
	return func(interface{}) ldvalue.Value { return value }
}

func FlagValueByTypeFactory() ValueFactory {
	return func(param interface{}) ldvalue.Value {
		valueType, _ := param.(servicedef.ValueType)
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

func DefaultValueByTypeFactory() ValueFactory {
	return func(param interface{}) ldvalue.Value {
		valueType, _ := param.(servicedef.ValueType)
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

func NewMemoizingFlagFactory(startingVersion int, factoryFn func(interface{}) ldmodel.FeatureFlag) FlagFactory {
	f := &MemoizingFlagFactory{
		factoryFn:   factoryFn,
		flags:       make(map[interface{}]ldmodel.FeatureFlag),
		nextVersion: startingVersion,
	}
	if f.nextVersion == 0 {
		f.nextVersion = 1
	}
	return f
}

func (f *MemoizingFlagFactory) MakeFlag(param interface{}) ldmodel.FeatureFlag {
	if flag, ok := f.flags[param]; ok {
		return flag
	}
	flag := f.factoryFn(param)
	f.nextVersion++
	flag.Version = f.nextVersion
	f.flags[param] = flag
	return flag
}

type FlagFactoryForValueTypes struct {
	KeyPrefix       string
	BuilderActions  func(*ldbuilders.FlagBuilder)
	ValueFactory    ValueFactory
	Reason          ldreason.EvaluationReason
	StartingVersion int
	factory         FlagFactory
}

func (f *FlagFactoryForValueTypes) ForType(valueType servicedef.ValueType) ldmodel.FeatureFlag {
	if f.factory == nil {
		if f.ValueFactory == nil {
			f.ValueFactory = FlagValueByTypeFactory()
		}
		f.factory = NewMemoizingFlagFactory(f.StartingVersion, func(param interface{}) ldmodel.FeatureFlag {
			valueType := param.(servicedef.ValueType)
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
	return f.factory.MakeFlag(valueType)
}
