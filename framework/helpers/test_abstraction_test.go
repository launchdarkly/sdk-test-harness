package helpers

import (
	"errors"
	"fmt"
	"strings"
)

// TestRecorder is a stub implementation of TestContext for testing test logic.
type TestRecorder struct {
	Errors           []string
	Terminated       bool
	PanicOnTerminate bool
}

func (t *TestRecorder) Errorf(format string, args ...interface{}) {
	t.Errors = append(t.Errors, fmt.Sprintf(format, args...))
}

func (t *TestRecorder) FailNow() {
	t.Terminated = true
	if t.PanicOnTerminate {
		panic(t)
	}
}

func (t *TestRecorder) Err() error {
	if len(t.Errors) == 0 {
		return nil
	}
	return errors.New(strings.Join(t.Errors, ", "))
}
