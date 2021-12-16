package matchers

import (
	"strings"
	"testing"
)

func TestNot(t *testing.T) {
	m := New(
		func(value interface{}) bool { return value == "good" },
		func(interface{}, DescribeValueFunc) string { return "should be good" },
	)
	assertPasses(t, "bad", Not(m))
	assertFails(t, "good", Not(m), "expected: not (should be good)\nactual value was: good")
}

func TestAllOf(t *testing.T) {
	m1 := New(
		func(value interface{}) bool { return strings.Contains(value.(string), "A") },
		func(interface{}, DescribeValueFunc) string { return "want A" },
	)
	m2 := New(
		func(value interface{}) bool { return strings.Contains(value.(string), "B") },
		func(interface{}, DescribeValueFunc) string { return "want B" },
	)
	assertPasses(t, "an A and a B", AllOf(m1, m2))
	assertFails(t, "a B", AllOf(m1, m2), "expected: want A\nactual value was: a B")
	assertFails(t, "an A", AllOf(m1, m2), "expected: want B\nactual value was: an A")
	assertFails(t, "a C", AllOf(m1, m2), "expected: (want A) and (want B)\nactual value was: a C")
}

func TestAnyOf(t *testing.T) {
	m1 := New(
		func(value interface{}) bool { return strings.Contains(value.(string), "A") },
		func(interface{}, DescribeValueFunc) string { return "want A" },
	)
	m2 := New(
		func(value interface{}) bool { return strings.Contains(value.(string), "B") },
		func(interface{}, DescribeValueFunc) string { return "want B" },
	)
	assertPasses(t, "an A and a B", AnyOf(m1, m2))
	assertPasses(t, "a B", AnyOf(m1, m2))
	assertPasses(t, "an A", AnyOf(m1, m2))
	assertFails(t, "a C", AnyOf(m1, m2), "expected: (want A) or (want B)\nactual value was: a C")
}
