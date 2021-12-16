package matchers

import "testing"

func TestEqual(t *testing.T) {
	assertPasses(t, 3, Equal(3))
	assertFails(t, 4, Equal(3), "expected: equal to 3\nactual value was: 4")

	assertPasses(t, map[string]interface{}{"a": []int{1, 2}},
		Equal(map[string]interface{}{"a": []int{1, 2}}))
}
