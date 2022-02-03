package ldtest

import (
	"fmt"
	"os"
	"strings"

	"github.com/launchdarkly/sdk-test-harness/framework"

	"github.com/fatih/color"
)

var consoleTestErrorColor = color.New(color.FgYellow)                   //nolint:gochecknoglobals
var consoleTestFailedColor = color.New(color.FgRed)                     //nolint:gochecknoglobals
var consoleTestFailedNonCriticalColor = color.New(color.FgYellow)       //nolint:gochecknoglobals
var consoleTestSkippedColor = color.New(color.Faint, color.FgBlue)      //nolint:gochecknoglobals
var consoleFailedDebugOutputColor = color.New(color.Faint, color.FgRed) //nolint:gochecknoglobals
var consolePassedDebugOutputColor = color.New(color.Faint)              //nolint:gochecknoglobals
var allTestsPassedColor = color.New(color.FgGreen)                      //nolint:gochecknoglobals

type TestLogger interface {
	TestStarted(id TestID)
	TestError(id TestID, err error)
	TestFinished(id TestID, result TestResult, debugOutput framework.CapturedOutput)
	TestSkipped(id TestID, reason string)
	EndLog(results Results) error
}

type nullTestLogger struct{}

func (n nullTestLogger) TestStarted(TestID)                                        {}
func (n nullTestLogger) TestError(TestID, error)                                   {}
func (n nullTestLogger) TestFinished(TestID, TestResult, framework.CapturedOutput) {}
func (n nullTestLogger) TestSkipped(TestID, string)                                {}
func (n nullTestLogger) EndLog(result Results) error                               { return nil }

type ConsoleTestLogger struct {
	DebugOutputOnFailure bool
	DebugOutputOnSuccess bool
}

type MultiTestLogger struct {
	Loggers []TestLogger
}

func (c ConsoleTestLogger) TestStarted(id TestID) {
	fmt.Printf("[%s]\n", id)
}

func (c ConsoleTestLogger) TestError(id TestID, err error) {
	for _, line := range strings.Split(err.Error(), "\n") {
		_, _ = consoleTestErrorColor.Printf("  %s\n", line)
	}
	if es, ok := err.(ErrorWithStacktrace); ok {
		_, _ = consoleTestErrorColor.Println("  Stacktrace:")
		for _, s := range es.Stacktrace {
			packageName := strings.TrimPrefix(s.Package, rootPackageName()+"/")
			_, _ = consoleTestErrorColor.Printf("    %s.%s (%s:%d)\n", packageName, s.Function, s.FileName, s.Line)
		}
	}
}

func (c ConsoleTestLogger) TestFinished(id TestID, result TestResult, debugOutput framework.CapturedOutput) {
	debugOutputColor := consolePassedDebugOutputColor
	if result.Failed() {
		debugOutputColor = consoleFailedDebugOutputColor
		if result.NonCritical {
			_, _ = consoleTestFailedNonCriticalColor.Printf("  FAILED (non-critical): %s\n", id)
			_, _ = consoleTestFailedNonCriticalColor.Printf("  Explanation: %s\n", result.Explanation)
		} else {
			_, _ = consoleTestFailedColor.Printf("  FAILED: %s\n", id)
		}
	}
	if len(debugOutput) > 0 &&
		((result.Failed() && c.DebugOutputOnFailure) || (!result.Failed() && c.DebugOutputOnSuccess)) {
		_, _ = debugOutputColor.Println(debugOutput.ToString("    DEBUG "))
	}
}

func (c ConsoleTestLogger) TestSkipped(id TestID, reason string) {
	if reason == "" {
		_, _ = consoleTestSkippedColor.Printf("  SKIPPED: %s\n", id)
	} else {
		_, _ = consoleTestSkippedColor.Printf("  SKIPPED: %s (%s)\n", id, reason)
	}
}

func (c ConsoleTestLogger) EndLog(results Results) error {
	if results.OK() {
		if len(results.NonCriticalFailures) == 0 {
			_, _ = allTestsPassedColor.Println("All tests passed")
		} else {
			_, _ = allTestsPassedColor.Println("All critical tests passed")
		}
	} else {
		_, _ = consoleTestFailedColor.Fprintln(os.Stderr, "Tests failed")
	}

	if len(results.NonCriticalFailures) != 0 {
		fmt.Fprintln(os.Stderr)
		_, _ = consoleTestFailedNonCriticalColor.Fprintf(os.Stderr,
			"NON-CRITICAL FAILURES (%d):\n", len(results.NonCriticalFailures))
		for _, f := range results.NonCriticalFailures {
			_, _ = consoleTestFailedNonCriticalColor.Fprintf(os.Stderr, "  * %s\n", f.TestID)
			_, _ = consoleTestFailedNonCriticalColor.Fprintf(os.Stderr, "      %s\n", f.Explanation)
		}
	}

	if !results.OK() {
		fmt.Fprintln(os.Stderr)
		_, _ = consoleTestFailedColor.Fprintf(os.Stderr, "FAILED TESTS (%d):\n", len(results.Failures))
		for _, f := range results.Failures {
			_, _ = consoleTestFailedColor.Fprintf(os.Stderr, "  * %s\n", f.TestID)
		}
	}

	return nil
}

func (m *MultiTestLogger) TestStarted(id TestID) {
	for _, l := range m.Loggers {
		l.TestStarted(id)
	}
}

func (m *MultiTestLogger) TestError(id TestID, err error) {
	for _, l := range m.Loggers {
		l.TestError(id, err)
	}
}

func (m *MultiTestLogger) TestFinished(id TestID, result TestResult, debugOutput framework.CapturedOutput) {
	for _, l := range m.Loggers {
		l.TestFinished(id, result, debugOutput)
	}
}

func (m *MultiTestLogger) TestSkipped(id TestID, reason string) {
	for _, l := range m.Loggers {
		l.TestSkipped(id, reason)
	}
}

func (m *MultiTestLogger) EndLog(results Results) error {
	for _, l := range m.Loggers {
		if err := l.EndLog(results); err != nil {
			return err
		}
	}
	return nil
}
