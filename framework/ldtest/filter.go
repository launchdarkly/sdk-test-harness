package ldtest

import (
	"fmt"
	"regexp"
	"strings"
)

// Filter is a function that can determine whether to run a specific test or not.
type Filter func(TestID) bool

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
		rx, err := regexp.Compile(part)
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

func PrintFilterDescription(filters RegexFilters, allCapabilities []string, supportedCapabilities []string) {
	if filters.MustMatch.IsDefined() || filters.MustNotMatch.IsDefined() {
		fmt.Println("Some tests will be skipped based on the filter criteria for this test run:")
		if filters.MustMatch.IsDefined() {
			fmt.Printf("  skip any not matching %s\n", filters.MustMatch)
		}
		if filters.MustNotMatch.IsDefined() {
			fmt.Printf("  skip any matching %s\n", filters.MustNotMatch)
		}
		fmt.Println()
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
			fmt.Println("Some tests may be skipped because the test service does not support the following capabilities:")
			fmt.Printf("  %s\n", strings.Join(missingCapabilities, ", "))
			fmt.Println()
		}
	}
}
