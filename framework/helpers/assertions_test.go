package helpers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func makePollTestFn[V any](initialValue, finalValue V, countBeforeFinalValue int) func() V {
	counter := 0
	return func() V {
		counter++
		if counter <= countBeforeFinalValue {
			return initialValue
		}
		return finalValue
	}

}

func TestPollForSpecificResultValue(t *testing.T) {
	t.Run("value is seen", func(t *testing.T) {
		assert.True(t, PollForSpecificResultValue(makePollTestFn("a", "b", 1), time.Second, time.Millisecond, "b"))
	})

	t.Run("value is not seen", func(t *testing.T) {
		assert.False(t, PollForSpecificResultValue(makePollTestFn("a", "b", 100), time.Millisecond*10, time.Millisecond, "b"))
	})
}

func TestEventually(t *testing.T) {
	t.Run("value is seen", func(t *testing.T) {
		var tr1 TestRecorder
		result := AssertEventually(&tr1, makePollTestFn(false, true, 1), time.Second, time.Millisecond, "sorry %s", "no")
		assert.True(t, result)
		assert.Len(t, tr1.Errors, 0)
		assert.False(t, tr1.Terminated)

		var tr2 TestRecorder
		RequireEventually(&tr2, makePollTestFn(false, true, 1), time.Second, time.Millisecond, "sorry %s", "no")
		assert.Len(t, tr2.Errors, 0)
		assert.False(t, tr2.Terminated)
	})

	t.Run("value is not seen", func(t *testing.T) {
		var tr1 TestRecorder
		result := AssertEventually(&tr1, makePollTestFn(false, true, 100), time.Millisecond*10, time.Millisecond, "sorry %s", "no")
		assert.False(t, result)
		if assert.Len(t, tr1.Errors, 1) {
			assert.Equal(t, "sorry no", tr1.Errors[0])
		}
		assert.False(t, tr1.Terminated)

		var tr2 TestRecorder
		RequireEventually(&tr2, makePollTestFn(false, true, 100), time.Millisecond*10, time.Millisecond, "sorry %s", "no")
		if assert.Len(t, tr2.Errors, 1) {
			assert.Equal(t, "sorry no", tr2.Errors[0])
		}
		assert.True(t, tr2.Terminated)
	})
}

func TestNever(t *testing.T) {
	t.Run("value is seen", func(t *testing.T) {
		var tr1 TestRecorder
		result := AssertNever(&tr1, makePollTestFn(false, true, 1), time.Second, time.Millisecond, "sorry %s", "no")
		assert.False(t, result)
		if assert.Len(t, tr1.Errors, 1) {
			assert.Equal(t, "sorry no", tr1.Errors[0])
			assert.False(t, tr1.Terminated)
		}

		var tr2 TestRecorder
		RequireNever(&tr2, makePollTestFn(false, true, 1), time.Second, time.Millisecond, "sorry %s", "no")
		if assert.Len(t, tr2.Errors, 1) {
			assert.Equal(t, "sorry no", tr2.Errors[0])
		}
		assert.True(t, tr2.Terminated)
	})

	t.Run("value is not seen", func(t *testing.T) {
		var tr1 TestRecorder
		result := AssertNever(&tr1, makePollTestFn(false, true, 100), time.Millisecond*10, time.Millisecond, "sorry %s", "no")
		assert.True(t, result)
		assert.Len(t, tr1.Errors, 0)
		assert.False(t, tr1.Terminated)

		var tr2 TestRecorder
		RequireNever(&tr2, makePollTestFn(false, true, 100), time.Millisecond*10, time.Millisecond, "sorry %s", "no")
		assert.Len(t, tr2.Errors, 0)
		assert.False(t, tr2.Terminated)
	})
}
