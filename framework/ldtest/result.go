package ldtest

import (
	"fmt"
	"strings"
)

type Results struct {
	Tests    []TestResult
	Failures []TestResult
}

type TestResult struct {
	TestID TestID
	Errors []error
}

func (r Results) OK() bool {
	return len(r.Failures) == 0
}

type TestID []string

func (t TestID) String() string {
	return strings.Join(t, "/")
}

func (t TestID) Plus(name string) TestID {
	return append(append(TestID(nil), t...), name)
}

type TestFailure struct {
	ID  TestID
	Err error
}

func (f TestFailure) Error() string {
	return fmt.Sprintf("[%s]: %s", f.ID, f.Err)
}
