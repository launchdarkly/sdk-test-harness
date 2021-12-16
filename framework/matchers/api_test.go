package matchers

import (
	"fmt"
	"testing"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type decoratedString string

func (s decoratedString) String() string { return decorate(string(s)) }

func decorate(value interface{}) string { return fmt.Sprintf("Hi, I'm '%s'", value.(string)) }

func assertPasses(t *testing.T, value interface{}, m Matcher) {
	pass, desc := m.Test(value)
	assert.True(t, pass)
	assert.Equal(t, "", desc)
}

func assertFails(t *testing.T, value interface{}, m Matcher, expectedDesc string) {
	pass, desc := m.Test(value)
	assert.False(t, pass)
	assert.Equal(t, expectedDesc, desc)
}

func TestSimpleMatcher(t *testing.T) {
	m := New(
		func(value interface{}) bool { return value == "good" },
		func(interface{}, DescribeValueFunc) string { return "should be good" },
	)
	assertPasses(t, "good", m)
	assertFails(t, "bad", m, "expected: should be good\nactual value was: bad")
}

func TestMatcherValueDescriptionUsesStringer(t *testing.T) {
	m := New(
		func(value interface{}) bool { return value == decoratedString("good") },
		func(interface{}, DescribeValueFunc) string { return "should be good" },
	)
	assertFails(t, decoratedString("bad"), m,
		fmt.Sprintf("expected: should be good\nactual value was: %s", decorate("bad")))
}

func TestAssertThat(t *testing.T) {
	result1 := ldtest.Run(ldtest.TestConfiguration{}, func(ldt *ldtest.T) {
		AssertThat(ldt, 2, Equal(2))
	})
	assert.True(t, result1.OK())

	result2 := ldtest.Run(ldtest.TestConfiguration{}, func(ldt *ldtest.T) {
		AssertThat(ldt, 3, Equal(2))
		AssertThat(ldt, 4, Equal(2))
	})
	assert.False(t, result2.OK())
	require.Len(t, result2.Failures[0].Errors, 2)
	assert.Contains(t, result2.Failures[0].Errors[0].Error(), "expected: equal to 2")
	assert.Contains(t, result2.Failures[0].Errors[0].Error(), "actual value was: 3")
	assert.Contains(t, result2.Failures[0].Errors[1].Error(), "expected: equal to 2")
	assert.Contains(t, result2.Failures[0].Errors[1].Error(), "actual value was: 4")
}

func TestRequireThat(t *testing.T) {
	result := ldtest.Run(ldtest.TestConfiguration{}, func(ldt *ldtest.T) {
		RequireThat(ldt, 3, Equal(2))
		RequireThat(ldt, 4, Equal(2))
	})
	assert.False(t, result.OK())
	require.Len(t, result.Failures[0].Errors, 1)
	assert.Contains(t, result.Failures[0].Errors[0].Error(), "actual value was: 3")
}

func TestEnsureType(t *testing.T) {
	m := New(
		func(value interface{}) bool { return value == "good" },
		func(interface{}, DescribeValueFunc) string { return "should be good" },
	)
	assertPasses(t, "good", m)
	assertFails(t, 3, m, "expected: should be good\nactual value was: 3")

	m1 := m.EnsureType("example string")
	assertPasses(t, "good", m1)
	assertFails(t, "bad", m1, "expected: should be good\nactual value was: bad")
	assertFails(t, 3, m1, "expected: value of type string, was int\nactual value was: 3")

	m2 := m.EnsureType(nil) // no-op
	assertPasses(t, "good", m2)
	assertFails(t, 3, m2, "expected: should be good\nactual value was: 3")
}

func TestWithValueDescription(t *testing.T) {
	m := New(
		func(value interface{}) bool { return value == "good" },
		func(value interface{}, desc DescribeValueFunc) string {
			return fmt.Sprintf("should be %s", desc("good"))
		},
	).WithValueDescription(decorate)

	assertPasses(t, "good", m)
	assertFails(t, "bad", m,
		fmt.Sprintf("expected: should be %s\nactual value was: %s", decorate("good"), decorate("bad")))
}
