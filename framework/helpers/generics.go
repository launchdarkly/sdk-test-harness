package helpers

import (
	"sort"

	"golang.org/x/exp/constraints"
)

// CopyOf returns a shallow copy of a slice.
func CopyOf[V any](slice []V) []V {
	return append([]V(nil), slice...)
}

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

// Sorted returns a sorted copy of a slice.
func Sorted[V constraints.Ordered](slice []V) []V {
	ret := CopyOf(slice)
	sort.Slice(ret, func(i, j int) bool { return ret[i] < ret[j] })
	return ret
}
