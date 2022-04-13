package helpers

import (
	"testing"
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/stretchr/testify/assert"
)

func TestNonBlockingSend(t *testing.T) {
	ch1 := make(chan string)
	assert.False(t, NonBlockingSend(ch1, "a"))

	ch2 := make(chan string, 1)
	assert.True(t, NonBlockingSend(ch2, "a"))
	assert.Equal(t, "a", <-ch2)
}

func TestTryReceive(t *testing.T) {
	ch := make(chan string, 1)
	assert.Equal(t, opt.None[string](), TryReceive(ch, time.Millisecond))

	ch <- "a"
	assert.Equal(t, opt.Some("a"), TryReceive(ch, time.Millisecond))

	go func() {
		time.Sleep(time.Millisecond * 50)
		ch <- "b"
	}()
	assert.Equal(t, opt.Some("b"), TryReceive(ch, time.Second))
}

func TestRequireValue(t *testing.T) {
	tr1 := TestRecorder{PanicOnTerminate: true}
	ch := make(chan string, 1)
	assert.PanicsWithValue(t, &tr1, func() { _ = RequireValue(&tr1, ch, time.Millisecond) })
	if assert.Error(t, tr1.Err()) {
		assert.Contains(t, tr1.Err().Error(), "waiting for value of type string")
	}

	tr2 := TestRecorder{PanicOnTerminate: true}
	ch <- "a"
	assert.Equal(t, "a", RequireValue(&tr2, ch, time.Millisecond))
	assert.NoError(t, tr2.Err())

	tr3 := TestRecorder{PanicOnTerminate: true}
	go func() {
		time.Sleep(time.Millisecond * 50)
		ch <- "b"
	}()
	assert.Equal(t, "b", RequireValue(&tr3, ch, time.Second))
	assert.NoError(t, tr3.Err())
}

func TestRequireNoMoreValues(t *testing.T) {
	tr1 := TestRecorder{PanicOnTerminate: true}
	ch := make(chan string, 1)
	RequireNoMoreValues(&tr1, ch, time.Millisecond)
	assert.NoError(t, tr1.Err())

	tr2 := TestRecorder{PanicOnTerminate: true}
	ch <- "a"
	assert.Panics(t, func() { RequireNoMoreValues(&tr2, ch, time.Millisecond) })
	if assert.Error(t, tr2.Err()) {
		assert.Contains(t, tr2.Err().Error(), "extra value of type string")
	}

	tr3 := TestRecorder{PanicOnTerminate: true}
	go func() {
		time.Sleep(time.Millisecond * 50)
		ch <- "b"
	}()
	assert.Panics(t, func() { RequireNoMoreValues(&tr3, ch, time.Second) })
	if assert.Error(t, tr3.Err()) {
		assert.Contains(t, tr3.Err().Error(), "extra value of type string")
	}
}
