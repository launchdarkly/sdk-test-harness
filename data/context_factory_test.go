package data

import (
	"testing"

	"gopkg.in/launchdarkly/go-sdk-common.v3/ldcontext"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldvalue"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/assert"
)

func TestContextFactory(t *testing.T) {
	f := NewContextFactory("abcde", func(b *ldcontext.Builder) {
		b.Name("x")
	})

	c1 := f.NextUniqueContext()
	assert.False(t, c1.Multiple())
	m.In(t).Assert(c1.Key(), m.StringHasPrefix("abcde."))
	assert.Equal(t, ldvalue.NewOptionalString("x"), c1.Name())

	c2 := f.NextUniqueContext()
	assert.False(t, c1.Multiple())
	m.In(t).Assert(c2.Key(), m.AllOf(
		m.StringHasPrefix("abcde."), m.Not(m.Equal(c1.Key()))))
	assert.Equal(t, ldvalue.NewOptionalString("x"), c2.Name())
}

func TestMultiContextFactory(t *testing.T) {
	f := NewMultiContextFactory("abcde", []ldcontext.Kind{"org", "other"}, func(b *ldcontext.Builder) {
		b.Name("x")
	})

	c1 := f.NextUniqueContext()
	assert.True(t, c1.Multiple())
	assert.Equal(t, 2, c1.MultiKindCount())
	c1a, _ := c1.MultiKindByIndex(0)
	assert.Equal(t, ldcontext.Kind("org"), c1a.Kind())
	m.In(t).Assert(c1a.Key(), m.StringHasPrefix("abcde."))
	assert.Equal(t, ldvalue.NewOptionalString("x"), c1a.Name())
	c1b, _ := c1.MultiKindByIndex(1)
	assert.Equal(t, ldcontext.Kind("other"), c1b.Kind())
	m.In(t).Assert(c1b.Key(), m.StringHasPrefix("abcde."))
	assert.Equal(t, ldvalue.NewOptionalString("x"), c1b.Name())

	c2 := f.NextUniqueContext()
	assert.True(t, c2.Multiple())
	assert.Equal(t, 2, c2.MultiKindCount())
	c2a, _ := c2.MultiKindByIndex(0)
	assert.Equal(t, ldcontext.Kind("org"), c2a.Kind())
	m.In(t).Assert(c2a.Key(), m.AllOf(
		m.StringHasPrefix("abcde."), m.Not(m.Equal(c1a.Key()))))
	assert.Equal(t, ldvalue.NewOptionalString("x"), c2a.Name())
	c2b, _ := c2.MultiKindByIndex(1)
	assert.Equal(t, ldcontext.Kind("other"), c2b.Kind())
	m.In(t).Assert(c2b.Key(), m.AllOf(
		m.StringHasPrefix("abcde."), m.Not(m.Equal(c1b.Key()))))
	assert.Equal(t, ldvalue.NewOptionalString("x"), c1b.Name())
}

func TestNewContextFactoriesForSingleAndMultiKind(t *testing.T) {
	fs := NewContextFactoriesForSingleAndMultiKind("abcde", func(b *ldcontext.Builder) {
		b.Name("x")
	})

	for _, f := range fs {
		c := f.NextUniqueContext()
		assert.NoError(t, c.Err())
	}
}
