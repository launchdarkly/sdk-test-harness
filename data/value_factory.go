package data

import (
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"gopkg.in/launchdarkly/go-sdk-common.v3/ldvalue"
)

// AllJSONValueTypes returns every possible value of the ldvalue.ValueType enum, corresponding to
// the standard JSON types (including null).
func AllJSONValueTypes() []ldvalue.ValueType {
	return []ldvalue.ValueType{ldvalue.NullType, ldvalue.BoolType, ldvalue.NumberType,
		ldvalue.StringType, ldvalue.ArrayType, ldvalue.ObjectType}
}

// AllSDKValueTypes returns every possible value of the servicedef.ValueType enum, corresponding to
// the logical types used in strongly-typed SDK APIs-- that is, the result types you could request
// when evaluating a flag. So, unlike AllJSONValueTypes, int and double are different, there is no
// null, and the "any" type is used for arbitrary JSON values.
func AllSDKValueTypes() []servicedef.ValueType {
	return []servicedef.ValueType{servicedef.ValueTypeBool, servicedef.ValueTypeInt,
		servicedef.ValueTypeDouble, servicedef.ValueTypeString, servicedef.ValueTypeAny}
}

// ValueFactoryBySDKValueType is a data generator function type that produces a different ldvalue.Value
// for each of the logical types supported by strongly-typed SDKs.
type ValueFactoryBySDKValueType func(valueType servicedef.ValueType) ldvalue.Value

// SingleValueForAllSDKValueTypes creates a ValueByTypeFactory generator that always returns the same value.
func SingleValueForAllSDKValueTypes(value ldvalue.Value) ValueFactoryBySDKValueType {
	return func(servicedef.ValueType) ldvalue.Value { return value }
}

// MakeValueFactoryBySDKValueType creates a ValueByTypeFactory generator providing a different value for
// each servicedef.ValueType.
func MakeValueFactoryBySDKValueType() ValueFactoryBySDKValueType {
	return MakeValueFactoriesBySDKValueType(1)[0]
}

// MakeValueFactoriesBySDKValueType creates the specified number of ValueByTypeFactory generators, each
// producing different values from the others (insofar as possible, given that there are only two
// possible values for the boolean type).
func MakeValueFactoriesBySDKValueType(howMany int) []ValueFactoryBySDKValueType {
	ret := make([]ValueFactoryBySDKValueType, 0, howMany)
	for i := 1; i <= howMany; i++ {
		index := i
		ret = append(ret, func(valueType servicedef.ValueType) ldvalue.Value {
			switch valueType {
			case servicedef.ValueTypeBool:
				return ldvalue.Bool(index%2 == 1)
			case servicedef.ValueTypeInt:
				return ldvalue.Int(index)
			case servicedef.ValueTypeDouble:
				return ldvalue.Float64(float64(index)*100 + 0.5)
			case servicedef.ValueTypeString:
				return ldvalue.String(fmt.Sprintf("string%d", index))
			default:
				return ldvalue.ObjectBuild().Set("a", ldvalue.Int(index)).Build()
			}
		})
	}
	return ret
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
