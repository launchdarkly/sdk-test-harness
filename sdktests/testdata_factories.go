package sdktests

import (
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldcontext"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldtime"
	"gopkg.in/launchdarkly/go-sdk-common.v3/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v2/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v2/ldmodel"
)

type ContextFactory struct {
	prefix         string
	counter        int
	builderActions []func(lduser.UserBuilder)
}

func NewUserFactory(prefix string, builderActions ...func(lduser.UserBuilder)) *ContextFactory {
	return &ContextFactory{
		prefix:         fmt.Sprintf("%s.%d", prefix, ldtime.UnixMillisNow()),
		builderActions: builderActions,
	}
}

func (f *ContextFactory) NextUniqueUser() ldcontext.Context {
	f.counter++
	key := fmt.Sprintf("%s.%d", f.prefix, f.counter)
	builder := lduser.NewUserBuilder(key)
	for _, ba := range f.builderActions {
		ba(builder)
	}
	return builder.Build()
}

func (f *ContextFactory) NextUniqueUserMaybeAnonymous(shouldBeAnonymous bool) ldcontext.Context {
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

// MakeStandardTestValues returns a list of values that cover all JSON types, *and* all special values
// that might conceivably be handled wrong in SDK implementations. For instance, we should check both
// non-empty and empty strings, and both zero and non-zero numbers, to make sure "" and 0 ares not being
// treated the same as "undefined/null".
func MakeStandardTestValues() []ldvalue.Value {
	return []ldvalue.Value{
		ldvalue.Null(),
		ldvalue.Bool(false),
		ldvalue.Bool(true),
		ldvalue.Int(-1000),
		ldvalue.Int(0),
		ldvalue.Int(1000),
		ldvalue.Float64(-1000.5),
		ldvalue.Float64(1000.5), // don't bother with Float64(0) because it is identical to Int(0)
		ldvalue.String(""),
		ldvalue.String("abc"),
		ldvalue.String("has \"escaped\" characters"),
		ldvalue.ArrayOf(),
		ldvalue.ArrayOf(ldvalue.String("a"), ldvalue.String("b")),
		ldvalue.ObjectBuild().Build(),
		ldvalue.ObjectBuild().Set("a", ldvalue.Int(1)).Build(),
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
