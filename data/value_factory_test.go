package data

import (
	"testing"

	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldvalue"

	"github.com/stretchr/testify/assert"
)

func equalsLDValue(value ldvalue.Value) m.Matcher { return m.JSONEqual(value) }

func TestSingleValueForAllSDKValueTypes(t *testing.T) {
	value := ldvalue.String("ok")
	f := SingleValueForAllSDKValueTypes(value)
	for _, valueType := range AllSDKValueTypes() {
		t.Run(string(valueType), func(t *testing.T) {
			m.In(t).Assert(f(valueType), equalsLDValue(value))
		})
	}
}

func assertJSONValueTypeMatchesSDKValueType(t *testing.T, value ldvalue.Value, desiredType servicedef.ValueType) {
	switch desiredType {
	case servicedef.ValueTypeBool:
		assert.Equal(t, ldvalue.BoolType, value.Type())
	case servicedef.ValueTypeInt, servicedef.ValueTypeDouble:
		assert.Equal(t, ldvalue.NumberType, value.Type())
	case servicedef.ValueTypeString:
		assert.Equal(t, ldvalue.StringType, value.Type())
	}
}

func TestMakeValueFactoriesBySDKValueType(t *testing.T) {
	factories := MakeValueFactoriesBySDKValueType(3)
	for _, valueType := range AllSDKValueTypes() {
		t.Run(string(valueType), func(t *testing.T) {
			for i, f := range factories {
				value := f(valueType)
				assertJSONValueTypeMatchesSDKValueType(t, value, valueType)
				atLeastOneUnequal := false
				for j, f1 := range factories {
					if j == i {
						continue
					}
					value1 := f1(valueType)
					if valueType == servicedef.ValueTypeBool {
						// for the boolean type, we can't require that *all* the values are unequal
						atLeastOneUnequal = atLeastOneUnequal || !value1.Equal(value)
					} else {
						atLeastOneUnequal = atLeastOneUnequal || m.In(t).Assert(value1, m.Not(equalsLDValue(value)))
					}
				}
				assert.True(t, atLeastOneUnequal)
			}
		})
	}
}

func TestMakeStandardTestValues(t *testing.T) {
	unusedTypes := make(map[ldvalue.ValueType]struct{})
	for _, vt := range AllJSONValueTypes() {
		unusedTypes[vt] = struct{}{}
	}
	for _, value := range MakeStandardTestValues() {
		delete(unusedTypes, value.Type())
	}
	assert.Len(t, unusedTypes, 0)
}
