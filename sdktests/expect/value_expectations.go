package expect

import (
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/framework/helpers"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"

	"github.com/stretchr/testify/assert"
)

// Value is the entry point for constructing assertions about ldvalue.Value values. While it is
// possible to use methods like assert.Equal with the ldvalue.Value type, the failure messages
// you get that way are very hard to read because assert does not respect ldvalue.Value's
// String() behavior.
//
//     var actualValue ldvalue.Value
//     Value.Equals(ldvalue.Bool(true)).Check(t, actualValue)
var Value ValueExpectationFactory //nolint:gochecknoglobals

type ValueExpectationFactory struct{}

type ValueExpectation struct {
	base helpers.Expectation
}

func (x ValueExpectation) Check(t assert.TestingT, v ldvalue.Value) bool {
	return x.base.Check(t, v)
}

func (x ValueExpectation) And(other ValueExpectation) ValueExpectation {
	return ValueExpectation{base: x.base.And(other.base)}
}

func (f ValueExpectationFactory) New(
	description string,
	fn func(t assert.TestingT, v ldvalue.Value) bool,
) ValueExpectation {
	base := helpers.NewExpectation(
		description,
		func(value interface{}) string {
			return value.(ldvalue.Value).JSONString()
		},
		func(t assert.TestingT, value interface{}) bool {
			return fn == nil || fn(t, value.(ldvalue.Value))
		},
	)
	return ValueExpectation{base}
}

func (f ValueExpectationFactory) Equals(expected ldvalue.Value) ValueExpectation {
	return f.New(fmt.Sprintf("value equals %s", expected.JSONString()),
		func(t assert.TestingT, v ldvalue.Value) bool {
			return helpers.AssertJSONEqual(t, expected.JSONString(), v.JSONString())
		})
}
