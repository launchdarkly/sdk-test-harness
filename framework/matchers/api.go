// Package matchers provides a flexible test assertion API similar to Java's Hamcrest. Matchers are
// constructed separately from the values being tested, and can then be applied to any value, or
// negated, or combined in various ways.
//
// This implementation is for Go 1.17 so it does not yet have generics. Instead, all matchers take
// values of type interface{} and must explicitly cast the type if needed. The simplest way to
// provide type safety is to use Matcher.EnsureType().
package matchers

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFunc is a function used in defining a new Matcher. It returns true if the value passes
// the test or false for failure.
type TestFunc func(value interface{}) bool

// DescribeFailureFunc is a function used in defining a new Matcher. Given the value that was
// tested, and assuming that the test failed, it returns a descriptive string.
//
// For simple conditions, this can just be a description of the test expectation (like, "equal
// to 3"); a description of the actual value will always be appended automatically. But it can
// use the value parameter if that will help to narrow down the nature of the failure.
//
// The second parameter is the function to use for making a string description of a value of
// the expected type.
type DescribeFailureFunc func(value interface{}, describeValueFunc DescribeValueFunc) string

// DescribeValueFunc is a function that can optionally be added to a Matcher. It returns a
// string description of the value. If you don't provide one, the default logic is
// DefaultDescription.
type DescribeValueFunc func(value interface{}) string

// Matcher is a general mechanism for declaring expectations about a value. Expectations can be combined,
// and they self-describe on failure.
type Matcher struct {
	maybeTest            TestFunc
	maybeDescribeFailure DescribeFailureFunc
	maybeDescribeValue   DescribeValueFunc
}

// New creates a Matcher.
func New(test TestFunc, describeFailure DescribeFailureFunc) Matcher {
	return Matcher{maybeTest: test, maybeDescribeFailure: describeFailure}
}

// Test executes the expectation for a specific value. It returns true if the value passes the
// test or false for failure, plus a string describing the expectation that failed.
func (m Matcher) Test(value interface{}) (pass bool, failDescription string) {
	if m.test(value) {
		return true, ""
	}
	testDesc := m.describeFailure(value, m.describeValue)
	return false, fmt.Sprintf("expected: %s\nactual value was: %s", testDesc, m.describeValue(value))
}

func (m Matcher) test(value interface{}) bool {
	if m.maybeTest == nil {
		return true
	}
	return m.maybeTest(value)
}

func (m Matcher) describeFailure(value interface{}, describeValue DescribeValueFunc) string {
	if m.maybeDescribeFailure == nil {
		return "no test description given"
	}
	return m.maybeDescribeFailure(value, describeValue)
}

func (m Matcher) describeValue(value interface{}) string {
	if m.maybeDescribeValue != nil {
		return m.maybeDescribeValue(value)
	}
	return DefaultDescription(value)
}

// Assert is for use with the testify/assert package (or any API with a compatible interface). It
// tests a value and, on failure, calls assert.Fail with the appropriate message.
func (m Matcher) Assert(t assert.TestingT, value interface{}) bool {
	if pass, desc := m.Test(value); !pass {
		assert.Fail(t, desc)
		return false
	}
	return true
}

// Require is for use with the testify/require package (or any API with a compatible interface). It
// tests a value and, on failure, calls require.Fail with the appropriate message.
func (m Matcher) Require(t require.TestingT, value interface{}) bool {
	if pass, desc := m.Test(value); !pass {
		require.Fail(t, desc)
		return false
	}
	return true
}

// EnsureType adds type safety to a matcher. The valueOfType parameter should be any value of the
// expected type. The returned Matcher will guarantee that the value is of that type before calling
// the original test function, so it is safe for the test function to cast the value.
func (m Matcher) EnsureType(valueOfType interface{}) Matcher {
	return New(
		func(value interface{}) bool {
			if valueOfType != nil && (reflect.TypeOf(value) != reflect.TypeOf(valueOfType)) {
				return false
			}
			return m.test(value)
		},
		func(value interface{}, desc DescribeValueFunc) string {
			if valueOfType != nil && reflect.TypeOf(value) != reflect.TypeOf(valueOfType) {
				return fmt.Sprintf("value of type %T, was %T", valueOfType, value)
			}
			return m.describeFailure(value, m.describeValue)
		},
	)
}

// WithValueDescription adds custom behavior for rendering the input value as a string in
// failure messages. If not specified, the default behavior is DefaultDescription. Another
// useful behavior is JSONDescription.
func (m Matcher) WithValueDescription(describeValue DescribeValueFunc) Matcher {
	ret := m
	ret.maybeDescribeValue = describeValue
	return ret
}

// DefaultDescription is the default behavior for rendering an input value as a string in
// failure messages. It checks whether the value implements the fmt.Stringer interface, and
// if so, calls its String method. If not, it calls fmt.Sprintf with the "%+v" format.
func DefaultDescription(value interface{}) string {
	if s, ok := value.(fmt.Stringer); ok {
		return s.String()
	}
	return fmt.Sprintf("%+v", value)
}

// JSONDescription is an optional behavior that can be passed to WithValueDescription. It
// renders the input value by calling JSON.Marshal on it.
func JSONDescription(value interface{}) string {
	data, _ := json.Marshal(value)
	return string(data)
}
