package helpers

import (
	"fmt"

	"github.com/stretchr/testify/assert"
)

// Expectation is a general mechanism for declaring expectations about a value. Expectations can be combined,
// and they self-describe on failure.
type Expectation struct {
	fn            func(assert.TestingT, interface{}) bool
	description   string
	describeValue func(interface{}) string
}

// NewExpectation creates an expectation from a function and a descriptive string.
//
// The function takes two parameters: some implementation of assert.TestingT (so functions from
// the assert and require packages can be used), and the value being tested. It should return
// true for success or false for failure (it can also generate assertion failures via assert or
// require, but should still return false for failure as well). The description is a string that
// will be printed in case of failure.
func NewExpectation(
	description string,
	describeValue func(interface{}) string,
	fn func(assert.TestingT, interface{}) bool,
) Expectation {
	return Expectation{fn: fn, description: description, describeValue: describeValue}
}

// Check executes the expectation for a specific value.
func (ex Expectation) Check(t assert.TestingT, value interface{}) bool {
	if ex.fn != nil {
		if !ex.fn(t, value) {
			valueDesc := ""
			if ex.describeValue != nil {
				valueDesc = ex.describeValue(value)
			} else {
				if s, ok := value.(fmt.Stringer); ok {
					valueDesc = s.String()
				} else {
					valueDesc = fmt.Sprintf("%+v", value)
				}
			}
			assert.Fail(t,
				fmt.Sprintf("failed condition was: %s\nactual value was: %s", ex.description, valueDesc))
			return false
		}
	}
	return true
}

// And combines two expectations. There is no short-circuiting-- all expectations will be executed even
// if the first fails.
func (ex Expectation) And(other Expectation) Expectation {
	ret := Expectation{
		fn: func(t assert.TestingT, value interface{}) bool {
			result1 := ex.fn == nil || ex.fn(t, value)
			result2 := other.fn == nil || other.fn(t, value)
			return result1 && result2
		},
		describeValue: ex.describeValue,
	}
	switch {
	case ex.description == "":
		ret.description = other.description
	case other.description == "":
		ret.description = ex.description
	default:
		ret.description = fmt.Sprintf("%s and %s", ex.description, other.description)
	}
	return ret
}
