package helpers

import "github.com/stretchr/testify/assert"

// AssertJSONEqual asserts that two JSON values are deeply equal, and, if they're not,
// prints a helpful diff.
//
// Currently this just delegates to assert.JSONEq. However, the output formatting of that
// function leaves something to be desired and we may replace it with smarter logic.
func AssertJSONEqual(t assert.TestingT, expectedJSONString, actualJSONString string) bool {
	return assert.JSONEq(t, expectedJSONString, actualJSONString)
}
