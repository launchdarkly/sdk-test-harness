package ldtest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest/internal"
)

func TestStacktrace(t *testing.T) {
	_ = Run(TestConfiguration{}, func(ldt *T) {
		ldt.Run("without filtering", func(ldt *T) {
			stack := getStacktrace(true, nil)
			assert.Greater(t, len(stack), 1)
			assert.Equal(t, currentPackageName(), stack[0].Package)
			assert.Contains(t, stack[0].Function, "TestStacktrace.")
			assert.Equal(t, currentPackageName(), stack[1].Package)
			assert.Equal(t, "(*T).run", stack[1].Function)
		})

		ldt.Run("auto-filtering removes ldtest methods", func(ldt *T) {
			internal.RunAction(func() {
				stack := getStacktrace(false, nil)
				assert.Len(t, stack, 1)
				// The ldtest stuff (including this test) and the Go runtime stuff below ldt.Run are
				// stripped out, leaving only internal.RunAction which isn't in ldtest.
				assert.Equal(t, currentPackageName()+"/internal", stack[0].Package)
				assert.Equal(t, "RunAction", stack[0].Function)
			})
		})

		ldt.Run("filter out designated helpers", func(ldt *T) {
			helperFunc1(func() {
				helperFunc2(func() {
					stack := getStacktrace(true, []string{currentPackageName() + ".helperFunc2"})
					foundFunc1 := false
					for _, s := range stack {
						if s.Package == currentPackageName() && s.Function == "helperFunc1" {
							foundFunc1 = true
						} else if s.Package == currentPackageName() && s.Function == "helperFunc2" {
							require.Fail(t, "helperFunc2 should not have been in stacktrace", "stacktrace: %+v", stack)
						}
					}
					assert.True(t, foundFunc1, "helperFunc1 should have been in stacktrace but wasn't", "stacktrace: +v", stack)
				})
			})
		})
	})
}

func helperFunc1(action func()) {
	action()
}

func helperFunc2(action func()) {
	action()
}
