package ldtest

import "fmt"

// SpecReference links a test to a formal specification requirement.
// It is the Go equivalent of the OpenFeature @Specification annotation (Java)
// or [Specification] attribute (.NET).
type SpecReference struct {
	SpecID  string // e.g. "HOOK", "DATASYSTEM"
	Number  string // e.g. "1.2.1"
	Summary string // e.g. "beforeEvaluation stage receives hook context"
}

// String returns a compact representation of the spec reference suitable for
// display in test output and JUnit XML properties.
func (s SpecReference) String() string {
	return fmt.Sprintf("%s %s: %s", s.SpecID, s.Number, s.Summary)
}
