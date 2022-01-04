package ldtest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTestIDString(t *testing.T) {
	assert.Equal(t, "", TestID{}.String())
	assert.Equal(t, "parent test", TestID{"parent test"}.String())
	assert.Equal(t, "parent test/subtest", TestID{"parent test", "subtest"}.String())
	assert.Equal(t, "parent test/subtest/sub-sub", TestID{"parent test", "subtest", "sub-sub"}.String())
}

func TestTestIDPlus(t *testing.T) {
	assert.Equal(t, TestID{"name 1"}, TestID{}.Plus("name 1"))
	assert.Equal(t, TestID{"name 1", "name 2"}, TestID{}.Plus("name 1").Plus("name 2"))

	// Calling Plus does not modify the original value
	id1 := TestID{"name 1"}
	id2a := id1.Plus("name 2a")
	id2b := id1.Plus("name 2b")
	assert.Equal(t, TestID{"name 1"}, id1)
	assert.Equal(t, TestID{"name 1", "name 2a"}, id2a)
	assert.Equal(t, TestID{"name 1", "name 2b"}, id2b)
}
