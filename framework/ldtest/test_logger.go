package ldtest

import (
	"fmt"
	"os"
	"strings"

	"github.com/launchdarkly/sdk-test-harness/framework"

	"github.com/fatih/color"
)

var consoleTestErrorColor = color.New(color.FgYellow)              //nolint:gochecknoglobals
var consoleTestFailedColor = color.New(color.FgRed)                //nolint:gochecknoglobals
var consoleTestSkippedColor = color.New(color.Faint, color.FgBlue) //nolint:gochecknoglobals
var consoleDebugOutputColor = color.New(color.Faint)               //nolint:gochecknoglobals
var allTestsPassedColor = color.New(color.FgGreen)                 //nolint:gochecknoglobals

type TestLogger interface {
	TestStarted(id TestID)
	TestError(id TestID, err error)
	TestFinished(id TestID, failed bool, debugOutput framework.CapturedOutput)
	TestSkipped(id TestID, reason string)
}

type nullTestLogger struct{}

func (n nullTestLogger) TestStarted(TestID)                                  {}
func (n nullTestLogger) TestError(TestID, error)                             {}
func (n nullTestLogger) TestFinished(TestID, bool, framework.CapturedOutput) {}
func (n nullTestLogger) TestSkipped(TestID, string)                          {}

type ConsoleTestLogger struct {
	DebugOutputOnFailure bool
	DebugOutputOnSuccess bool
}

func (c ConsoleTestLogger) TestStarted(id TestID) {
	fmt.Printf("[%s]\n", id)
}

func (c ConsoleTestLogger) TestError(id TestID, err error) {
	for _, line := range strings.Split(err.Error(), "\n") {
		_, _ = consoleTestErrorColor.Printf("  %s\n", line)
	}
}

func (c ConsoleTestLogger) TestFinished(id TestID, failed bool, debugOutput framework.CapturedOutput) {
	if failed {
		_, _ = consoleTestFailedColor.Printf("  FAILED: %s\n", id)
	}
	if len(debugOutput) > 0 &&
		((failed && c.DebugOutputOnFailure) || (!failed && c.DebugOutputOnSuccess)) {
		_, _ = consoleDebugOutputColor.Println(debugOutput.ToString("    DEBUG "))
	}
}

func (c ConsoleTestLogger) TestSkipped(id TestID, reason string) {
	if reason == "" {
		_, _ = consoleTestSkippedColor.Printf("  SKIPPED: %s\n", id)
	} else {
		_, _ = consoleTestSkippedColor.Printf("  SKIPPED: %s (%s)\n", id, reason)
	}
}

func PrintResults(results Results) {
	if results.OK() {
		_, _ = allTestsPassedColor.Println("All tests passed")
	} else {
		_, _ = consoleTestFailedColor.Fprintf(os.Stderr, "FAILED TESTS (%d):\n", len(results.Failures))
		for _, f := range results.Failures {
			_, _ = consoleTestFailedColor.Fprintf(os.Stderr, "  * %s\n", f.TestID)
		}
	}
}
