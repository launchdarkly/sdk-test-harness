package framework

// Capabilities is a type alias for a list of strings representing test service capabilities. The
// meanings of these strings are defined by the domain-specific test service spec.
type Capabilities []string

// Has returns true if the specified string appears in the list.
func (cs Capabilities) Has(name string) bool {
	for _, c := range cs {
		if c == name {
			return true
		}
	}
	return false
}
