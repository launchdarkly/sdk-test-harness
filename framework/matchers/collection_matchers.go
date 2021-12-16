package matchers

import (
	"fmt"
	"reflect"
)

// ItemsInAnyOrder is a matcher for a slice value. It tests that the slice contains the same number of
// elements as the number of parameters, and that each parameter is a matcher that matches one item in
// the slice.
//
//     s := []int{6,2}
//     matchers.ItemsInAnyOrder(matchers.Equal(2), matchers.Equal(6)).Test(s) // pass
func ItemsInAnyOrder(matchers ...Matcher) Matcher {
	return New(
		func(value interface{}) bool {
			v := reflect.ValueOf(value)
			if v.Type().Kind() != reflect.Slice {
				return false
			}
			if v.Len() != len(matchers) {
				return false
			}
			foundCount := 0
			for _, m := range matchers {
				for j := 0; j < v.Len(); j++ {
					if m.test(v.Index(j).Interface()) {
						foundCount++
						break
					}
				}
			}
			return foundCount == len(matchers)
		},
		func(value interface{}, desc DescribeValueFunc) string {
			// It should be possible to make a better failure message where it lists the specific
			// matchers that weren't found, and/or the non-matched items. That will be particularly
			// helpful for lists of events. For now, it's just spitting out the whole condition.
			v := reflect.ValueOf(value)
			if v.Type().Kind() != reflect.Slice {
				return "a slice"
			}
			if v.Len() != len(matchers) {
				return fmt.Sprintf("should have %d item(s) (had %d)", len(matchers), v.Len())
			}
			return "contains in any order: " + describeMatchersList(matchers, value, ", ")
		},
	)
}
