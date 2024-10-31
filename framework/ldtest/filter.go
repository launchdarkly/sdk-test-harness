package ldtest

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
)

// Filter is an object that can determine whether to run a specific test or not.
type Filter interface {
	Match(id TestID) bool
}

type SelfDescribingFilter interface {
	Describe(out io.Writer, supportedCapabilities, importantCapabilities []string) string
}

type FilterFunc func(id TestID) bool

func (f FilterFunc) Match(id TestID) bool {
	return f(id)
}

type RegexFilters struct {
	MustMatch    TestIDPatternList
	MustNotMatch TestIDPatternList
}

func (r RegexFilters) Match(id TestID) bool {
	return (!r.MustMatch.IsDefined() || r.MustMatch.AnyMatch(id, true)) &&
		!r.MustNotMatch.AnyMatch(id, false)
}

type TestIDPattern []*regexp.Regexp

func (p TestIDPattern) Match(id TestID, includeParents bool) bool {
	min := len(p)
	if min > len(id) {
		if !includeParents {
			return false
		}
		min = len(id)
	}
	for i := 0; i < min; i++ {
		if !p[i].MatchString(id[i]) {
			return false
		}
	}
	return true
}

func (p TestIDPattern) String() string {
	ss := make([]string, 0, len(p))
	for _, c := range p {
		ss = append(ss, c.String())
	}
	return strings.Join(ss, "/")
}

func ParseTestIDPattern(s string) (TestIDPattern, error) {
	parts := strings.Split(s, "/")
	ret := make(TestIDPattern, 0, len(parts))
	for _, part := range parts {
		rx, err := regexp.Compile(autoEscapeTestRegex(part))
		if err != nil {
			return nil, fmt.Errorf("invalid regex: %w", err)
		}
		ret = append(ret, rx)
	}
	return ret, nil
}

type TestIDPatternList []TestIDPattern

func (l TestIDPatternList) String() string {
	ss := make([]string, 0, len(l))
	for _, p := range l {
		ss = append(ss, `"`+p.String()+`"`)
	}
	return strings.Join(ss, " or ")
}

// Set is called by the command line parser
func (l *TestIDPatternList) Set(value string) error {
	p, err := ParseTestIDPattern(value)
	if err != nil {
		return err
	}
	*l = append(*l, p)
	return nil
}

func (l TestIDPatternList) IsDefined() bool {
	return len(l) != 0
}

func (l TestIDPatternList) AnyMatch(id TestID, includeParents bool) bool {
	for _, p := range l {
		if p.Match(id, includeParents) {
			return true
		}
	}
	return false
}

func (r RegexFilters) Describe(out io.Writer, supportedCapabilities, allCapabilities []string) {
	if r.MustMatch.IsDefined() || r.MustNotMatch.IsDefined() {
		helpers.MustFprintln(out, "Some tests will be skipped based on the filter criteria for this test run:")
		if r.MustMatch.IsDefined() {
			helpers.MustFprintf(out, "  skip any not matching %s\n", r.MustMatch)
		}
		if r.MustNotMatch.IsDefined() {
			helpers.MustFprintf(out, "  skip any matching %s\n", r.MustNotMatch)
		}
		helpers.MustFprintln(out)
	}

	if len(supportedCapabilities) != 0 {
		supported := make(map[string]bool)
		for _, c := range supportedCapabilities {
			supported[c] = true
		}
		var missingCapabilities []string
		for _, c := range allCapabilities {
			if !supported[c] {
				missingCapabilities = append(missingCapabilities, c)
			}
		}
		if len(missingCapabilities) > 0 {
			helpers.MustFprintln(
				out,
				"Some tests may be skipped because the test service does not support the following capabilities:",
			)
			helpers.MustFprintf(out, "  %s\n", strings.Join(missingCapabilities, ", "))
			helpers.MustFprintln(out)
		}
	}
}

func autoEscapeTestRegex(pattern string) string {
	s := pattern
	for _, ch := range []string{"(", ")"} {
		s = strings.ReplaceAll(s, ch, "\\"+ch)
		s = strings.ReplaceAll(s, "\\\\"+ch, "\\"+ch) // in case they already escaped it
	}
	return s
}
