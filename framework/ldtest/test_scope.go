package ldtest

import (
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/launchdarkly/sdk-test-harness/v2/framework"
)

type environment struct {
	config  TestConfiguration
	results Results
}

// T represents a test scope. It is very similar to Go's testing.T type.
type T struct {
	env         *environment
	id          TestID
	debugLogger framework.CapturingLogger
	nonCritical string
	failed      bool
	skipped     bool
	skipReason  string
	cleanups    []func()
	errors      []error
	helperFns   []string
}

// TestConfiguration contains options for the entire test run.
type TestConfiguration struct {
	// Filter is an optional function for determining which tests to run based on their names.
	Filter Filter

	// TestLogger receives status information about each test.
	TestLogger TestLogger

	// Context is an optional value of any type defined by the application which can be accessed from tests.
	Context interface{}

	// Capabilities is a list of strings which are used by T.HasCapability and T.RequireCapability.
	Capabilities []string
}

func (t TestConfiguration) WithContext(context interface{}) TestConfiguration {
	t.Context = context
	return t
}

// Run starts a top-level test scope.
func Run(
	config TestConfiguration,
	action func(*T),
) Results {
	if config.TestLogger == nil {
		config.TestLogger = nullTestLogger{}
	}
	env := &environment{
		config: config,
	}
	t := &T{env: env}
	t.run(action)
	return env.results
}

func (t *T) run(action func(*T)) (result TestResult) {
	result.TestID = t.id
	defer func() {
		if r := recover(); r != nil {
			if t.skipped {
				return
			}
			t.failed = true
			var addError error
			if _, ok := r.(*T); ok {
				if len(t.errors) == 0 {
					addError = errors.New("test failed with no failure message")
				}
			} else {
				addError = fmt.Errorf("unexpected panic in test: %+v\n%s", r, string(debug.Stack()))
			}
			if addError != nil {
				t.errors = append(t.errors, addError)
				t.env.config.TestLogger.TestError(t.id, addError)
			}
		}
		result.Errors = t.errors
		if t.failed {
			if t.nonCritical == "" {
				t.env.results.Failures = append(t.env.results.Failures, result)
			} else {
				result.Explanation = t.nonCritical
				result.NonCritical = true
				t.env.results.NonCriticalFailures = append(t.env.results.NonCriticalFailures, result)
			}
		}
		t.env.results.Tests = append(t.env.results.Tests, result)
		for i := len(t.cleanups) - 1; i >= 0; i-- {
			t.cleanups[i]()
		}
	}()

	action(t)
	return result
}

// ID returns the full name of the current test.
func (t *T) ID() TestID {
	return t.id
}

// Run runs a subtest in its own scope.
//
// This is equivalent to Go's testing.T.Run.
func (t *T) Run(name string, action func(*T)) {
	id := t.id.Plus(name)

	t.env.config.TestLogger.TestStarted(id)
	if t.env.config.Filter != nil && !t.env.config.Filter.Match(id) {
		t.env.config.TestLogger.TestSkipped(id, "excluded by filter parameters")
		return
	}
	c1 := &T{
		id:  id,
		env: t.env,
	}
	t.debugLogger.AddChildLogger(&c1.debugLogger) // see comments on t.DebugLogger()
	result := c1.run(action)
	t.debugLogger.RemoveChildLogger(&c1.debugLogger)
	if c1.skipped {
		t.env.config.TestLogger.TestSkipped(id, c1.skipReason)
	} else {
		t.env.config.TestLogger.TestFinished(id, result, c1.debugLogger.Output())
	}
}

// NonCritical indicates that if this test fails, we would like to know about it but we're willing to
// live with it. It will be shown in the output as a non-critical failure, accompanied by the
// explanation that is specified here. Non-critical failures do not cause sdk-test-harness to return
// a non-zero exit code on termination, as regular failures do.
func (t *T) NonCritical(explanation string) {
	t.nonCritical = explanation
}

// Errorf reports a test failure. It is equivalent to Go's testing.T.Errorf. It does not cause the test
// to terminate, but adds the failure message to the output and marks the test as failed.
//
// You will rarely use this method directly; it is part of this type's implementation of the base
// interfaces testing.T and assert.TestingT, allowing it to be called from assertion helpers.
func (t *T) Errorf(format string, args ...interface{}) {
	t.failed = true
	err := fmt.Errorf(format, args...)

	stacktrace := getStacktrace(false, t.helperFns)
	err = transformError(err, stacktrace)

	t.errors = append(t.errors, err)
	t.env.config.TestLogger.TestError(t.id, err)
}

// FailNow causes the test to immediately terminate and be marked as failed.
//
// You will rarely use this method directly; it is part of this type's implementation of the base
// interfaces testing.T and assert.TestingT, allowing it to be called from assertion helpers.
func (t *T) FailNow() {
	panic(t)
}

// Skip causes the test to immediately terminate and be marked as skipped.
func (t *T) Skip() {
	t.skipped = true
	panic(t)
}

// SkipWithReason is equivalent to Skip but provides a message.
func (t *T) SkipWithReason(reason string) {
	t.skipReason = reason
	t.Skip()
}

// Debug writes a message to the output for this test scope.
func (t *T) Debug(message string, args ...interface{}) {
	t.debugLogger.Printf(message, args...)
}

// DebugLogger returns a Logger instance for writing output for this test scope.
//
// The output that is captured for a test will be passed to TestLogger.TestFinished at the end of
// the test. The test runner can choose whether to display this or not based on command-line options.
//
// When a test has subtests (created with t.Run), the logger for a subtest starts out with a copy of
// any output that was already logged for the parent test. During the lifetime of the subtest, any
// further output that is sent to the parent test's logger will go to the child test's logger
// instead. This is useful when the parent test scope manages an object such as a mock endpoint that
// is reused by many subtests.
func (t *T) DebugLogger() framework.Logger {
	return &t.debugLogger
}

// Defer schedules a cleanup function which is guaranteed to be called when this test scope
// exits for any reason. Unlike a Go defer statement, Defer can be used from within helper
// functions.
func (t *T) Defer(cleanupFn func()) {
	t.cleanups = append(t.cleanups, cleanupFn)
}

// Context returns the application-defined context value, if any, that was specified in the
// TestConfiguration.
func (t *T) Context() interface{} {
	return t.env.config.Context
}

func (t *T) WithContext(context interface{}) *T {
	copied := *t
	copiedEnv := *t.env
	copiedEnv.config = copiedEnv.config.WithContext(context)
	copied.env = &copiedEnv
	return &copied
}

// Capabilities returns the capabilities reported by the test service.
func (t *T) Capabilities() framework.Capabilities {
	return append(framework.Capabilities(nil), t.env.config.Capabilities...)
}

// RequireCapability causes the test to be skipped if HasCapability(name) returns false.
func (t *T) RequireCapability(name string) {
	if !t.Capabilities().Has(name) {
		t.SkipWithReason(fmt.Sprintf("test service does not have capability %q", name))
	}
}

// Helper marks the function that calls it as a test helper that shouldn't appear in stacktraces.
// Equivalent to Go's testing.T.Helper().
func (t *T) Helper() {
	pc, _, _, ok := runtime.Caller(1) // 0 is Helper() itself, 1 is who called it
	if !ok {
		return
	}
	f := runtime.FuncForPC(pc)
	if f == nil {
		return
	}
	t.helperFns = append(t.helperFns, f.Name())
}
