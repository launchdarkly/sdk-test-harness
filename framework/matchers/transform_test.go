package matchers

import (
	"fmt"
	"testing"
)

func stringLength() MatcherTransform {
	return Transform(
		"string length",
		func(value interface{}) interface{} { return len(value.(string)) },
	)
}

func TestTransform(t *testing.T) {
	m := stringLength().Should(Equal(3))

	assertPasses(t, "abc", m)
	assertFails(t, "abcd", m, "expected: string length equal to 3\nactual value was: abcd")
}

func TestTransformEnsureType(t *testing.T) {
	m := stringLength().EnsureInputValueType("example string").
		Should(Equal(3))

	assertPasses(t, "abc", m)
	assertFails(t, "abcd", m, "expected: string length equal to 3\nactual value was: abcd")
	assertFails(t, 3, m, "expected: value of type string, was int\nactual value was: 3")
}

func TestTransformInputValueDesc(t *testing.T) {
	m := stringLength().WithInputValueDescription(decorate).
		Should(Equal(3))

	assertPasses(t, "abc", m)
	assertFails(t, "abcd", m,
		fmt.Sprintf("expected: string length equal to 3\nactual value was: %s", decorate("abcd")))
}

func TestTransformOutputValueDesc(t *testing.T) {
	decorateInt := func(value interface{}) string { return fmt.Sprintf("the number %d", value) }

	m := stringLength().WithOutputValueDescription(decorateInt).
		Should(Equal(3))

	assertPasses(t, "abc", m)
	assertFails(t, "abcd", m,
		"expected: string length equal to the number 3\nactual value was: abcd")
}
