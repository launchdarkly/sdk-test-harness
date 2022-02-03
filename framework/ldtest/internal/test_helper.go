// Package internal contains test helpers for ldtest.
package internal

// RunAction is used only in unit tests, but exported because it has to be in a separate package for test purposes
func RunAction(action func()) {
	action()
}

// RunAction2 is used only in unit tests, but exported because it has to be in a separate package for test purposes
func RunAction2(action func()) {
	action()
}
