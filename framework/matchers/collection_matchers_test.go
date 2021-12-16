package matchers

import "testing"

func TestItemsInAnyOrder(t *testing.T) {
	slice := []string{"y", "z", "x"}

	assertPasses(t, slice, ItemsInAnyOrder(Equal("x"), Equal("y"), Equal("x")))
	assertPasses(t, slice, ItemsInAnyOrder(Equal("y"), Equal("z"), Equal("x")))

	assertFails(t, slice, ItemsInAnyOrder(Equal("x"), Equal("y")),
		"expected: should have 2 item(s) (had 3)\nactual value was: [y z x]")

	assertFails(t, slice, ItemsInAnyOrder(Equal("x"), Equal("a"), Equal("z")),
		"expected: contains in any order: (equal to x), (equal to a), (equal to z)"+
			"\nactual value was: [y z x]")
}
