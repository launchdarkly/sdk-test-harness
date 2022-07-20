package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCopyOf(t *testing.T) {
	s := []string{"a", "b"}
	s1 := CopyOf(s)
	assert.Equal(t, s, s1)
	s[0] = "x"
	assert.Equal(t, "a", s1[0])
}

func TestIfElse(t *testing.T) {
	assert.Equal(t, 3, IfElse(true, 3, 4))
	assert.Equal(t, 4, IfElse(false, 3, 4))
	assert.Equal(t, "a", IfElse(true, "a", "b"))
	assert.Equal(t, "b", IfElse(false, "a", "b"))
}

func TestSliceContains(t *testing.T) {
	assert.True(t, SliceContains(3, []int{1, 2, 3, 4}))
	assert.False(t, SliceContains(5, []int{1, 2, 3, 4}))
	assert.True(t, SliceContains("c", []string{"a", "b", "c", "d"}))
	assert.False(t, SliceContains("e", []string{"a", "b", "c", "d"}))
}

func TestSorted(t *testing.T) {
	s := []string{"d", "a", "c", "b"}
	s1 := Sorted(s)
	assert.Equal(t, []string{"a", "b", "c", "d"}, s1)
	assert.Equal(t, []string{"d", "a", "c", "b"}, s)
}
