package ldtest

import (
	"testing"

	"github.com/launchdarkly/sdk-test-harness/framework"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest/internal"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestScopeInheritsConfiguration(t *testing.T) {
	myContextValue := "hi"
	myCapabilities := framework.Capabilities{"a", "b"}
	config := TestConfiguration{
		Context:      myContextValue,
		Capabilities: myCapabilities,
	}
	_ = Run(config, func(ldt *T) {
		assert.Equal(t, myContextValue, ldt.Context())
		assert.Equal(t, myCapabilities, ldt.Capabilities())

		ldt.Run("subtest", func(ldt1 *T) {
			assert.Equal(t, myContextValue, ldt1.Context())
			assert.Equal(t, myCapabilities, ldt1.Capabilities())
		})
	})
}

func TestTestScopeExitsImmediatelyOnFailNow(t *testing.T) {
	executed1 := false
	executed2 := false
	executed3 := false
	_ = Run(TestConfiguration{}, func(ldt *T) {
		ldt.Run("", func(ldt *T) {
			executed1 = true
			ldt.FailNow()
			executed2 = true
		})
		executed3 = true
	})
	assert.True(t, executed1)
	assert.False(t, executed2)
	assert.True(t, executed3)
}

func TestTestScopeExitsImmediatelyOnSkip(t *testing.T) {
	executed1 := false
	executed2 := false
	executed3 := false
	_ = Run(TestConfiguration{}, func(ldt *T) {
		ldt.Run("", func(ldt *T) {
			executed1 = true
			ldt.Skip()
			executed2 = true
		})
		executed3 = true
	})
	assert.True(t, executed1)
	assert.False(t, executed2)
	assert.True(t, executed3)
}

func TestTestScopePassedResult(t *testing.T) {
	result := Run(TestConfiguration{}, func(ldt *T) {
		ldt.Run("parent", func(ldt0 *T) {
			ldt0.Run("subtest1", func(ldt1 *T) {
				// this test passes
			})
			ldt0.Run("subtest2", func(ldt2 *T) {
				// this test passes
			})
		})
	})

	assert.True(t, result.OK())
	assert.Len(t, result.Tests, 4)
	assert.Len(t, result.Failures, 0)

	assert.Equal(t, TestID{"parent", "subtest1"}, result.Tests[0].TestID)
	assert.Len(t, result.Tests[0].Errors, 0)

	assert.Equal(t, TestID{"parent", "subtest2"}, result.Tests[1].TestID)
	assert.Len(t, result.Tests[1].Errors, 0)

	assert.Equal(t, TestID{"parent"}, result.Tests[2].TestID)
	assert.Len(t, result.Tests[2].Errors, 0)

	assert.Nil(t, result.Tests[3].TestID)
	assert.Len(t, result.Tests[3].Errors, 0)
}

func TestTestScopeFailedResult(t *testing.T) {
	result := Run(TestConfiguration{}, func(ldt *T) {
		ldt.Run("parent", func(ldt0 *T) {
			ldt0.Run("subtest1", func(ldt1 *T) {
				// this test passes
			})
			ldt0.Run("subtest2", func(ldt2 *T) {
				ldt2.Errorf("failed because %s", "reasons")
				ldt2.Errorf("and failed some more")
			})
			ldt0.Errorf("and parent failed")
		})
	})

	assert.False(t, result.OK())
	assert.Len(t, result.Tests, 4)
	assert.Len(t, result.Failures, 2)

	assert.Equal(t, TestID{"parent", "subtest1"}, result.Tests[0].TestID)
	assert.Len(t, result.Tests[0].Errors, 0)

	assert.Equal(t, TestID{"parent", "subtest2"}, result.Tests[1].TestID)
	assert.Len(t, result.Tests[1].Errors, 2)
	assert.Equal(t, "failed because reasons", result.Tests[1].Errors[0].Error())
	assert.Equal(t, "and failed some more", result.Tests[1].Errors[1].Error())

	assert.Equal(t, TestID{"parent"}, result.Tests[2].TestID)
	assert.Len(t, result.Tests[2].Errors, 1)
	assert.Equal(t, "and parent failed", result.Tests[2].Errors[0].Error())

	assert.Nil(t, result.Tests[3].TestID)
	assert.Len(t, result.Tests[3].Errors, 0)
}

func TestTestScopeSkippedResult(t *testing.T) {
	result := Run(TestConfiguration{}, func(ldt *T) {
		ldt.Run("parent", func(ldt0 *T) {
			ldt0.Run("subtest1", func(ldt1 *T) {
				ldt1.Skip()
			})
			ldt0.Run("subtest2", func(ldt2 *T) {
				ldt2.SkipWithReason("why not")
			})
		})
	})

	assert.True(t, result.OK())
	assert.Len(t, result.Tests, 2)
	assert.Len(t, result.Failures, 0)

	assert.Equal(t, TestID{"parent"}, result.Tests[0].TestID)
	assert.Len(t, result.Tests[0].Errors, 0)

	assert.Nil(t, result.Tests[1].TestID)
	assert.Len(t, result.Tests[1].Errors, 0)
}

func TestTestScopeFilter(t *testing.T) {
	filter := FilterFunc(func(id TestID) bool {
		return len(id) == 0 || id[0] == "b"
	})

	result := Run(TestConfiguration{Filter: filter}, func(ldt *T) {
		ldt.Run("a", func(ldt0 *T) {
			ldt0.Run("sub1a", func(ldt1 *T) {})
			ldt0.Run("sub2a", func(ldt1 *T) {})
		})
		ldt.Run("b", func(ldt0 *T) {
			ldt0.Run("sub1b", func(ldt1 *T) {})
			ldt0.Run("sub2b", func(ldt1 *T) {})
		})
	})

	assert.True(t, result.OK())
	assert.Len(t, result.Tests, 4)
	assert.Len(t, result.Failures, 0)

	assert.Equal(t, TestID{"b", "sub1b"}, result.Tests[0].TestID)
	assert.Equal(t, TestID{"b", "sub2b"}, result.Tests[1].TestID)
	assert.Equal(t, TestID{"b"}, result.Tests[2].TestID)
	assert.Equal(t, TestID(nil), result.Tests[3].TestID)
}

func TestNonCriticalFailure(t *testing.T) {
	result := Run(TestConfiguration{}, func(ldt *T) {
		ldt.Run("a", func(ldt0 *T) {
			ldt0.NonCritical("would be nice if this worked (and it does)")
		})

		ldt.Run("b", func(ldt1 *T) {
			ldt1.NonCritical("would be nice if this worked")
			ldt1.Errorf("but it doesn't")
		})
	})

	assert.True(t, result.OK())
	assert.Len(t, result.Tests, 3)

	assert.Equal(t, TestID{"a"}, result.Tests[0].TestID)
	assert.Len(t, result.Tests[0].Errors, 0)
	assert.Equal(t, "", result.Tests[0].Explanation)

	assert.Equal(t, TestID{"b"}, result.Tests[1].TestID)
	assert.Len(t, result.Tests[1].Errors, 1)
	assert.Equal(t, "but it doesn't", result.Tests[1].Errors[0].Error())
	assert.Equal(t, "would be nice if this worked", result.Tests[1].Explanation)

	assert.Equal(t, TestID(nil), result.Tests[2].TestID)

	assert.Len(t, result.Failures, 0)

	assert.Len(t, result.NonCriticalFailures, 1)
	assert.Equal(t, TestID{"b"}, result.NonCriticalFailures[0].TestID)
	assert.Equal(t, "would be nice if this worked", result.NonCriticalFailures[0].Explanation)
	assert.Len(t, result.NonCriticalFailures[0].Errors, 1)
	assert.Equal(t, "but it doesn't", result.NonCriticalFailures[0].Errors[0].Error())
}

func TestFailureStacktrace(t *testing.T) {
	t.Run("stacktrace is captured", func(t *testing.T) {
		result := Run(TestConfiguration{}, func(ldt *T) {
			internal.RunAction(func() { // RunAction is there just so it'll show up in the stacktrace
				ldt.Errorf("sorry")
			})
		})
		require.Len(t, result.Failures, 1)
		require.Len(t, result.Failures[0].Errors, 1)
		err := result.Failures[0].Errors[0]
		if assert.IsType(t, ErrorWithStacktrace{}, err) {
			es := err.(ErrorWithStacktrace)
			assert.Equal(t, "sorry", es.Error())
			require.Len(t, es.Stacktrace, 1)
			assert.Equal(t, "RunAction", es.Stacktrace[0].Function)
		}
	})

	t.Run("helpers are filtered out", func(t *testing.T) {
		result := Run(TestConfiguration{}, func(ldt *T) {
			internal.RunAction(func() {
				// The assert functions all call Helper() if it is available
				assert.Fail(ldt, "sorry")
			})
		})
		require.Len(t, result.Failures, 1)
		require.Len(t, result.Failures[0].Errors, 1)
		err := result.Failures[0].Errors[0]
		if assert.IsType(t, ErrorWithStacktrace{}, err) {
			es := err.(ErrorWithStacktrace)
			assert.Equal(t, "sorry", es.Error())
			assert.Greater(t, len(es.Stacktrace), 0)
			for _, s := range es.Stacktrace {
				assert.NotEqual(t, "github.com/stretchr/testify/assert", s.Package,
					"assert functions should not appear in stacktrace due to using t.Helper(); stacktrace: %+v",
					es.Stacktrace)
			}
		}
	})
}

func TestParentTestLoggerIsCopiedToChildTestLogger(t *testing.T) {
	outputLines := func(ldt *T) []string {
		var ret []string
		for _, m := range ldt.debugLogger.Output() {
			ret = append(ret, m.Message)
		}
		return ret
	}

	Run(TestConfiguration{}, func(ldt *T) {
		ldt.DebugLogger().Println("parent log 1")

		ldt.Run("child1", func(ldt1 *T) {
			ldt1.DebugLogger().Println("child1 log 1")
			ldt.DebugLogger().Println("parent log 2")
			ldt1.DebugLogger().Println("child1 log 2")

			assert.Equal(t, []string{
				"parent log 1", "child1 log 1", "parent log 2", "child1 log 2",
			}, outputLines(ldt1))
		})

		ldt.Run("child2", func(ldt2 *T) {
			ldt2.DebugLogger().Println("child2 log 1")
			ldt.DebugLogger().Println("parent log 3")
			ldt2.DebugLogger().Println("child2 log 2")

			assert.Equal(t, []string{
				"parent log 1", "child2 log 1", "parent log 3", "child2 log 2",
			}, outputLines(ldt2))
		})

		ldt.DebugLogger().Println("parent log 4")
		assert.Equal(t, []string{
			"parent log 1", "parent log 4",
		}, outputLines(ldt))
	})
}
