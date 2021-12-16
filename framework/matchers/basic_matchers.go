package matchers

import (
	"fmt"
	"reflect"
)

// Equal is a matcher that tests whether the input value matches the expected value according
// to reflect.DeepEqual.
func Equal(expectedValue interface{}) Matcher {
	return New(
		func(value interface{}) bool {
			return reflect.DeepEqual(value, expectedValue)
		},
		func(value interface{}, desc DescribeValueFunc) string {
			return fmt.Sprintf("equal to %s", desc(expectedValue))
		},
	)
}
