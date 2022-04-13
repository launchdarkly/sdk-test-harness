package helpers

// IfElse returns valueIfTrue or valueIfFalse depending on isTrue.
func IfElse[V any](isTrue bool, valueIfTrue, valueIfFalse V) V {
	if isTrue {
		return valueIfTrue
	}
	return valueIfFalse
}

// SliceContains returns true if and only if the slice has an element that equals the value.
func SliceContains[V comparable](value V, slice []V) bool {
	for _, element := range slice {
		if element == value {
			return true
		}
	}
	return false
}
