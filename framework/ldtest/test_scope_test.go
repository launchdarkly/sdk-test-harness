package ldtest

import (
	"testing"

	"github.com/launchdarkly/sdk-test-harness/framework"
	"github.com/stretchr/testify/assert"
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
	filter := func(id TestID) bool {
		return len(id) == 0 || id[0] == "b"
	}

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
