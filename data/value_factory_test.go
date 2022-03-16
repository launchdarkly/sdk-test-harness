package data

import (
	"fmt"
	"testing"

	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"gopkg.in/launchdarkly/go-sdk-common.v3/ldvalue"

	"github.com/stretchr/testify/assert"
)

var allValueTypes = []ldvalue.ValueType{ldvalue.NullType, ldvalue.BoolType, ldvalue.NumberType,
	ldvalue.StringType, ldvalue.ArrayType, ldvalue.ObjectType}

func TestFlagValueFactory(t *testing.T) {
	fvf := FlagValueByTypeFactory()
	dvf := DefaultValueByTypeFactory()

	for _, p := range []struct {
		valueType   servicedef.ValueType
		ldValueType ldvalue.ValueType
	}{
		{servicedef.ValueTypeBool, ldvalue.BoolType},
		{servicedef.ValueTypeInt, ldvalue.NumberType},
		{servicedef.ValueTypeDouble, ldvalue.NumberType},
		{servicedef.ValueTypeString, ldvalue.StringType},
		{servicedef.ValueTypeAny, ldvalue.ObjectType},
	} {
		t.Run(fmt.Sprintf("%v", p.valueType), func(t *testing.T) {
			fv := fvf(p.valueType)
			dv := dvf(p.valueType)
			assert.Equal(t, p.ldValueType, fv.Type())
			assert.Equal(t, p.ldValueType, dv.Type())
			assert.NotEqual(t, fv, dv)
		})
	}
}
