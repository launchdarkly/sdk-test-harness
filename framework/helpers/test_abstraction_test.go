package helpers

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTestRecorder(t *testing.T) {
	t.Run("Errorf", func(t *testing.T) {
		var tr TestRecorder
		tr.Errorf("hello %s", "there")
		tr.Errorf("bye")
		assert.Equal(t, []string{"hello there", "bye"}, tr.Errors)
		assert.False(t, tr.Terminated)
	})

	t.Run("FailNow", func(t *testing.T) {
		var tr1 TestRecorder
		tr1.FailNow()
		assert.True(t, tr1.Terminated)

		tr2 := TestRecorder{PanicOnTerminate: true}
		assert.Panics(t, func() { tr2.FailNow() })
		assert.True(t, tr2.Terminated)
	})

	t.Run("Err", func(t *testing.T) {
		var tr TestRecorder
		assert.Nil(t, tr.Err())

		tr.Errorf("hello %s", "there")
		tr.Errorf("bye")
		assert.Equal(t, errors.New("hello there, bye"), tr.Err())
	})
}
