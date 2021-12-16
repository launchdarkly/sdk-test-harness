package matchers

import (
	"fmt"
	"strings"
)

// Not negates the result of another Matcher.
//
//     matchers.Not(Equal(3)).Assert(t, 4)
//     // failure message will describe expectation as "not (equal to 3)"
func Not(matcher Matcher) Matcher {
	return New(
		func(value interface{}) bool {
			return !matcher.test(value)
		},
		func(value interface{}, desc DescribeValueFunc) string {
			return fmt.Sprintf("not (%s)", matcher.describeFailure(value, matcher.describeValue))
		},
	).WithValueDescription(matcher.describeValue)
}

// AllOf requires that the input value passes all of the specified Matchers. If it fails,
// the failure message describes all of the Matchers that failed.
func AllOf(matchers ...Matcher) Matcher {
	var describeValueFn func(interface{}) string
	if len(matchers) != 0 {
		describeValueFn = matchers[0].describeValue
	}
	return New(
		func(value interface{}) bool {
			for _, m := range matchers {
				if !m.test(value) {
					return false
				}
			}
			return true
		},
		func(value interface{}, desc DescribeValueFunc) string {
			var fails []Matcher
			for _, m := range matchers {
				if !m.test(value) {
					fails = append(fails, m)
				}
			}
			return describeMatchersList(fails, value, " and ")
		},
	).WithValueDescription(describeValueFn)
}

// AnyOf requires that the input value does not fail any of the specified Matchers. If it fails,
// the failure message describes all of the Matchers that failed.
func AnyOf(matchers ...Matcher) Matcher {
	var describeValueFn func(interface{}) string
	if len(matchers) != 0 {
		describeValueFn = matchers[0].describeValue
	}
	return New(
		func(value interface{}) bool {
			for _, m := range matchers {
				if m.test(value) {
					return true
				}
			}
			return false
		},
		func(value interface{}, desc DescribeValueFunc) string {
			var fails []Matcher
			for _, m := range matchers {
				if !m.test(value) {
					fails = append(fails, m)
				}
			}
			return describeMatchersList(fails, value, " or ")
		},
	).WithValueDescription(describeValueFn)
}

func describeMatchersList(matchers []Matcher, value interface{}, separator string) string {
	if len(matchers) == 1 {
		return matchers[0].describeFailure(value, matchers[0].describeValue)
	}
	parts := make([]string, 0, len(matchers))
	for _, m := range matchers {
		parts = append(parts, "("+m.describeFailure(value, m.describeValue)+")")
	}
	return strings.Join(parts, separator)
}
