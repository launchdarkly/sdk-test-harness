package sdktests

import (
	"testing"

	"github.com/launchdarkly/sdk-test-harness/v3/framework"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errorRecordingTestLogger records the errors that the test suite reports.
type errorRecordingTestLogger struct {
	errors []error
}

func (l *errorRecordingTestLogger) TestStarted(ldtest.TestID) {}
func (l *errorRecordingTestLogger) TestError(_ ldtest.TestID, err error) {
	l.errors = append(l.errors, err)
}
func (l *errorRecordingTestLogger) TestFinished(ldtest.TestID, ldtest.TestResult, framework.CapturedOutput) {
}
func (l *errorRecordingTestLogger) TestSkipped(ldtest.TestID, string) {}
func (l *errorRecordingTestLogger) EndLog(ldtest.Results) error       { return nil }

func TestTestSuiteReportsWhyItCannotRun(t *testing.T) {
	// The fake test service declares no capabilities, so it is neither a client-side nor a
	// server-side SDK. The summary of failed tests shows only test names, so the reason must
	// go to the test logger.
	h := newTestHarnessForDataSystemTest(t)
	logger := &errorRecordingTestLogger{}

	results := RunSDKTestSuite(h, nil, logger, false)

	require.Len(t, results.Failures, 1)
	require.Len(t, logger.errors, 1, "the test logger must receive the reason that the suite cannot run")
	assert.Contains(t, logger.errors[0].Error(), `neither "client-side" nor "server-side"`)
}
