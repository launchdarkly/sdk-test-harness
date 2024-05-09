package framework

import "golang.org/x/exp/slices"

// Capabilities is a type alias for a list of strings representing test service capabilities. The
// meanings of these strings are defined by the domain-specific test service spec.
type Capabilities []string

// Has returns true if the specified string appears in the list.
func (cs Capabilities) Has(name string) bool {
	return slices.Contains(cs, name)
}

// HasAny returns true if any of the specified strings appear in the list.
func (cs Capabilities) HasAny(names ...string) bool {
	caps := make(map[string]struct{})
	for _, c := range cs {
		caps[c] = struct{}{}
	}
	for _, name := range names {
		if _, ok := caps[name]; ok {
			return true
		}
	}
	return false
}
