package helpers

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework/opt"
)

// NonBlockingSend is a shortcut for using select to do a non-blocking send. It returns
// true on success or false if the channel was full.
func NonBlockingSend[V any](ch chan<- V, value V) bool {
	select {
	case ch <- value:
		return true
	default:
		return false
	}
}

// TryReceive is a shortcut for using select to do a receive with timeout. It returns a
// Maybe that has a value if one was available, or no value if it timed out.
func TryReceive[V any](ch <-chan V, timeout time.Duration) opt.Maybe[V] {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	select {
	case value := <-ch:
		return opt.Some(value)
	case <-deadline.C:
		return opt.None[V]()
	}
}

// RequireValue tries to receive a value and returns it if successful, or causes the test
// to fail and terminate immediately if it timed out.
func RequireValue[V any](t TestContext, ch <-chan V, timeout time.Duration) V {
	t.Helper()
	var empty V
	return RequireValueWithMessage(t, ch, timeout, "timed out waiting for value of type %T", empty)
}

// RequireValueWithMessage is the same as RequireValue, but allows customization of the failure message.
func RequireValueWithMessage[V any](
	t TestContext,
	ch <-chan V,
	timeout time.Duration,
	msgFormat string,
	msgArgs ...interface{},
) V {
	t.Helper()
	maybeValue := TryReceive(ch, timeout)
	if !maybeValue.IsDefined() {
		t.Errorf(msgFormat, msgArgs...)
		t.FailNow()
	}
	return maybeValue.Value()
}

// RequireNoMoreValues tries to receive a value within the given timeout, and causes the test
// to fail and terminate immediately if a value was received.
func RequireNoMoreValues[V any](t TestContext, ch <-chan V, timeout time.Duration) {
	t.Helper()
	var empty V
	RequireNoMoreValuesWithMessage(t, ch, timeout, "received unexpected extra value of type %T", empty)
}

// RequireNoMoreValuesWithMessage is the same as RequireNoMoreValues, but allows customization
// of the failure message.
func RequireNoMoreValuesWithMessage[V any](
	t TestContext,
	ch <-chan V,
	timeout time.Duration,
	msgFormat string,
	msgArgs ...interface{},
) {
	t.Helper()
	maybeValue := TryReceive(ch, timeout)
	if maybeValue.IsDefined() {
		t.Errorf(msgFormat, msgArgs...)
		t.FailNow()
	}
}
