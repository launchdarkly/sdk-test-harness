package ldtest

import (
	"encoding/xml"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v3/framework"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/harness"
	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
)

type JUnitTestLogger struct {
	filePath    string
	serviceInfo harness.TestServiceInfo
	filters     RegexFilters
	testIDs     []TestID // this slice preserves the order that the tests were run in
	tests       map[string]jUnitTestStatus
	lock        sync.Mutex
}

type jUnitTestStatus struct {
	failures    []error
	skipped     o.Maybe[string]
	nonCritical bool
	output      string
	startTime   time.Time
	duration    time.Duration
}

// Struct definitions for the JUnit XML schema - see https://github.com/jstemmer/go-junit-report

type jUnitXMLDocument struct {
	XMLName xml.Name            `xml:"testsuites"`
	Suites  []jUnitXMLTestSuite `xml:"testsuite"`
}

type jUnitXMLTestSuite struct {
	XMLName    xml.Name           `xml:"testsuite"`
	Tests      int                `xml:"tests,attr"`
	Failures   int                `xml:"failures,attr"`
	Time       string             `xml:"time,attr"`
	Name       string             `xml:"name,attr"`
	Properties []jUnitXMLProperty `xml:"properties>property,omitempty"`
	TestCases  []jUnitXMLTestCase `xml:"testcase"`
}

type jUnitXMLTestCase struct {
	XMLName     xml.Name             `xml:"testcase"`
	Classname   string               `xml:"classname,attr"`
	Name        string               `xml:"name,attr"`
	Time        string               `xml:"time,attr"`
	SkipMessage *jUnitXMLSkipMessage `xml:"skipped,omitempty"`
	Failure     *jUnitXMLFailure     `xml:"failure,omitempty"`
}

type jUnitXMLSkipMessage struct {
	Message string `xml:"message,attr"`
}

type jUnitXMLProperty struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type jUnitXMLFailure struct {
	Message  string `xml:"message,attr"`
	Type     string `xml:"type,attr"`
	Contents string `xml:",chardata"`
}

func NewJUnitTestLogger(
	filePath string,
	serviceInfo harness.TestServiceInfo,
	filters RegexFilters,
) *JUnitTestLogger {
	return &JUnitTestLogger{
		filePath:    filePath,
		serviceInfo: serviceInfo,
		filters:     filters,
		tests:       make(map[string]jUnitTestStatus),
	}
}

func (j *JUnitTestLogger) TestStarted(id TestID) {
	j.lock.Lock()
	defer j.lock.Unlock()
	j.testIDs = append(j.testIDs, id)
	j.tests[id.String()] = jUnitTestStatus{
		startTime: time.Now(),
	}
}

func (j *JUnitTestLogger) TestError(id TestID, err error) {
	j.lock.Lock()
	defer j.lock.Unlock()
	status := j.tests[id.String()]
	status.failures = append(status.failures, err)
	j.tests[id.String()] = status
}

func (j *JUnitTestLogger) TestFinished(id TestID, result TestResult, debugOutput framework.CapturedOutput) {
	j.lock.Lock()
	defer j.lock.Unlock()
	status := j.tests[id.String()]
	status.output = debugOutput.ToString("")
	status.duration = time.Since(status.startTime)
	status.nonCritical = result.NonCritical
	j.tests[id.String()] = status
}

func (j *JUnitTestLogger) TestSkipped(id TestID, reason string) {
	j.lock.Lock()
	defer j.lock.Unlock()
	status := j.tests[id.String()]
	status.skipped = o.Some(reason)
	j.tests[id.String()] = status
}

func (j *JUnitTestLogger) EndLog(results Results) error {
	fmt.Printf("Writing JUnit data to %s\n", j.filePath)

	var doc jUnitXMLDocument

	properties := []jUnitXMLProperty{
		{
			Name:  "tests.service.info",
			Value: string(j.serviceInfo.FullData),
		},
		{
			Name:  "tests.filter.mustMatch",
			Value: j.filters.MustMatch.String(),
		},
		{
			Name:  "tests.filter.mustNotMatch",
			Value: j.filters.MustNotMatch.String(),
		},
	}

	for _, topLevelID := range getTopLevelIDs(j.testIDs) {
		suite := jUnitXMLTestSuite{
			Name:       fmt.Sprintf("SDK contract tests: %s", topLevelID),
			Properties: properties,
		}
		suiteTotalDuration := time.Duration(0)
		for _, testID := range j.testIDs {
			if len(testID) == 0 || testID[0] != topLevelID {
				continue
			}
			status := j.tests[testID.String()]

			suite.Tests++
			if len(status.failures) != 0 {
				suite.Failures++
			}
			suiteTotalDuration += status.duration

			testCase := jUnitXMLTestCase{
				Name: testID.String(),
				Time: jUnitDurationString(status.duration),
			}
			if status.nonCritical {
				testCase.Name += " (non-critical)"
			}
			if status.skipped.IsDefined() {
				testCase.SkipMessage = &jUnitXMLSkipMessage{Message: status.skipped.Value()}
			}
			if len(status.failures) != 0 {
				var messages []string
				for _, e := range status.failures {
					message := e.Error()
					if es, ok := e.(ErrorWithStacktrace); ok {
						message += "\n  Stacktrace:"
						for _, s := range es.Stacktrace {
							message += "\n    " + s.String()
						}
					}
					messages = append(messages, message)
				}
				testCase.Failure = &jUnitXMLFailure{
					Message:  strings.Join(messages, "\n"),
					Contents: status.output,
				}
			}

			suite.TestCases = append(suite.TestCases, testCase)
		}
		suite.Time = jUnitDurationString(suiteTotalDuration)
		doc.Suites = append(doc.Suites, suite)
	}

	bytes, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	bytes = append(bytes, '\n')

	return os.WriteFile(j.filePath, bytes, 0644) //nolint:gosec
}

func getTopLevelIDs(allIDs []TestID) []string {
	var ret []string
	seen := make(map[string]bool)
	for _, testID := range allIDs {
		if len(testID) != 0 && !seen[testID[0]] {
			ret = append(ret, testID[0])
			seen[testID[0]] = true
		}
	}
	return ret
}

func jUnitDurationString(d time.Duration) string {
	return fmt.Sprintf("%.3f", d.Seconds())
}
