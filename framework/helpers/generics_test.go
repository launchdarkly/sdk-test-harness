package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
