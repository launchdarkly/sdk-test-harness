package helpers

import (
	"time"
)

// PollForSpecificResultValue calls testFn repeatedly at intervals until the expected value is seen or the timeout
// elapses.
// Returns true if the value was matched, false if timed out.
func PollForSpecificResultValue[V comparable](
	testFn func() V,
	timeout time.Duration,
	interval time.Duration,
	expectedValue V,
) bool {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case <-deadline.C:
			return false
		case <-ticker.C:
			if testFn() == expectedValue {
				return true
			}
		}
	}
}

// AssertEventually is equivalent to assert.Eventually from stretchr/testify/assert, except that it does not use a
// separate goroutine so it does not cause problems with our test framework. It calls testFn
// repeatedly at intervals until it gets a true value; if the timeout elapses, the test fails.
func AssertEventually(
	t TestContext,
	testFn func() bool,
	timeout time.Duration,
	interval time.Duration,
	failureMsgFormat string,
	failureMsgArgs ...interface{},
) bool {
	if PollForSpecificResultValue(testFn, timeout, interval, true) {
		return true
	}
	t.Errorf(failureMsgFormat, failureMsgArgs...)
	return false
}

// RequireEventually is equivalent to require.Eventually from stretchr/testify/assert, except that it does not use a
// separate goroutine so it does not cause problems with our test framework. It calls testFn
// repeatedly at intervals until it gets a true value; if the timeout elapses, the test fails
// and immediately exits.
func RequireEventually(
	t TestContext,
	testFn func() bool,
	timeout time.Duration,
	interval time.Duration,
	failureMsgFormat string,
	failureMsgArgs ...interface{},
) {
	if !AssertEventually(t, testFn, timeout, interval, failureMsgFormat, failureMsgArgs...) {
		t.FailNow()
	}
}

// AssertNever is equivalent to assert.Never from stretchr/testify/assert, except that it does not use a
// separate goroutine so it does not cause problems with our test framework. It calls testFn
// repeatedly at intervals until either the timeout elapses or it receives a true value; if
// it receives a true value, the test fails.
func AssertNever(
	t TestContext,
	testFn func() bool,
	timeout time.Duration,
	interval time.Duration,
	failureMsgFormat string,
	failureMsgArgs ...interface{},
) bool {
	if PollForSpecificResultValue(testFn, timeout, interval, true) {
		t.Errorf(failureMsgFormat, failureMsgArgs...)
		return false
	}
	return true
}

// RequireNever is equivalent to require.Never from stretchr/testify/assert, except that it does not use a
// separate goroutine so it does not cause problems with our test framework. It calls testFn
// repeatedly at intervals until either the timeout elapses or it receives a true value; if
// it receives a true value, the test fails and exits immediately
func RequireNever(
	t TestContext,
	testFn func() bool,
	timeout time.Duration,
	interval time.Duration,
	failureMsgFormat string,
	failureMsgArgs ...interface{},
) {
	if !AssertNever(t, testFn, timeout, interval, failureMsgFormat, failureMsgArgs...) {
		t.FailNow()
	}
}
