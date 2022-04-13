package helpers

import (
	"errors"
	"fmt"
	"strings"
)

// TestContext is a minimal interface for types like *testing.T and *ldtest.T representing a
// test that can fail. Functions can use this to avoid specific dependencies on those packages.
type TestContext interface {
	Errorf(msgFormat string, msgArgs ...interface{})
	FailNow()
}

// TestRecorder is a stub implementation of TestContext for testing test logic.
type TestRecorder struct {
	Errors           []string
	Terminated       bool
	PanicOnTerminate bool
}

// Errorf records an error message.
func (t *TestRecorder) Errorf(format string, args ...interface{}) {
	t.Errors = append(t.Errors, fmt.Sprintf(format, args...))
}

// FailNow simulates a condition where the test is supposed to immediately terminate. It sets
// t.Terminated to true, and then does a panic(t) if and only if t.PanicOnTerminate is true.
func (t *TestRecorder) FailNow() {
	t.Terminated = true
	if t.PanicOnTerminate {
		panic(t)
	}
}

// Err returns an error whose message is a concatenation of all error messages so far, or nil
// if none.
func (t *TestRecorder) Err() error {
	if len(t.Errors) == 0 {
		return nil
	}
	return errors.New(strings.Join(t.Errors, ", "))
}
