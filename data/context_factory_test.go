package data

import (
	"testing"

	"gopkg.in/launchdarkly/go-sdk-common.v3/ldcontext"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldvalue"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func equalsOptString(s string) m.Matcher       { return m.Equal(ldvalue.NewOptionalString(s)) }
func optStringEmpty() m.Matcher                { return m.Equal(ldvalue.OptionalString{}) }
func equalsKind(kind ldcontext.Kind) m.Matcher { return m.Equal(kind) }

func setContextName(name string) func(*ldcontext.Builder) {
	return func(b *ldcontext.Builder) { b.Name(name) }
}
func setContextSecondary(value string) func(*ldcontext.Builder) {
	return func(b *ldcontext.Builder) { b.Secondary(value) }
}

func TestContextFactory(t *testing.T) {
	f := NewContextFactory("abcde",
		setContextName("x"),
		setContextSecondary("y"),
	)

	c1 := f.NextUniqueContext()
	assert.False(t, c1.Multiple())
	m.In(t).Assert(c1.Kind(), equalsKind(ldcontext.DefaultKind))
	m.In(t).Assert(c1.Key(), m.StringHasPrefix("abcde."))
	m.In(t).Assert(c1.Name(), equalsOptString("x"))
	m.In(t).Assert(c1.Secondary(), equalsOptString("y"))

	c2 := f.NextUniqueContext()
	assert.False(t, c2.Multiple())
	m.In(t).Assert(c2.Kind(), equalsKind(ldcontext.DefaultKind))
	m.In(t).Assert(c2.Key(), m.AllOf(
		m.StringHasPrefix("abcde."), m.Not(m.Equal(c1.Key()))))
	m.In(t).Assert(c2.Name(), equalsOptString("x"))
	m.In(t).Assert(c2.Secondary(), equalsOptString("y"))
}

func TestMultiContextFactory(t *testing.T) {
	f := NewMultiContextFactory("abcde", []ldcontext.Kind{"org", "other"},
		setContextName("x"),
		setContextSecondary("y"),
	)

	c1 := f.NextUniqueContext()
	assert.True(t, c1.Multiple())
	assert.Equal(t, 2, c1.MultiKindCount())
	c1a, _ := c1.MultiKindByIndex(0)
	m.In(t).Assert(c1a.Kind(), equalsKind("org"))
	m.In(t).Assert(c1a.Key(), m.StringHasPrefix("abcde."))
	m.In(t).Assert(c1a.Name(), equalsOptString("x"))
	m.In(t).Assert(c1a.Secondary(), optStringEmpty())
	c1b, _ := c1.MultiKindByIndex(1)
	m.In(t).Assert(c1b.Kind(), equalsKind("other"))
	m.In(t).Assert(c1b.Key(), m.StringHasPrefix("abcde."))
	m.In(t).Assert(c1b.Name(), optStringEmpty())
	m.In(t).Assert(c1b.Secondary(), equalsOptString("y"))

	c2 := f.NextUniqueContext()
	assert.True(t, c2.Multiple())
	assert.Equal(t, 2, c2.MultiKindCount())
	c2a, _ := c2.MultiKindByIndex(0)
	m.In(t).Assert(c2a.Kind(), equalsKind("org"))
	m.In(t).Assert(c2a.Key(), m.AllOf(
		m.StringHasPrefix("abcde."), m.Not(m.Equal(c1a.Key()))))
	m.In(t).Assert(c2a.Name(), equalsOptString("x"))
	m.In(t).Assert(c2a.Secondary(), optStringEmpty())
	c2b, _ := c2.MultiKindByIndex(1)
	m.In(t).Assert(c2b.Kind(), equalsKind("other"))
	m.In(t).Assert(c2b.Key(), m.AllOf(
		m.StringHasPrefix("abcde."), m.Not(m.Equal(c1b.Key()))))
	m.In(t).Assert(c2b.Name(), optStringEmpty())
	m.In(t).Assert(c2b.Secondary(), equalsOptString("y"))
}

func TestNewContextFactoriesForAnonymousAndNonAnonymous(t *testing.T) {
	fs := NewContextFactoriesForAnonymousAndNonAnonymous("abcde",
		setContextName("x"),
		setContextSecondary("y"),
	)
	require.Len(t, fs, 2)
	fNonAnon, fAnon := fs[0], fs[1]
	assert.Equal(t, "non-anonymous user", fNonAnon.Description())
	assert.Equal(t, "anonymous user", fAnon.Description())

	c1a := fNonAnon.NextUniqueContext()
	assert.False(t, c1a.Transient())
	m.In(t).Assert(c1a.Key(), m.StringHasPrefix("abcde"))
	c1b := fNonAnon.NextUniqueContext()
	assert.False(t, c1b.Transient())
	m.In(t).Assert(c1b.Key(), m.AllOf(m.StringHasPrefix("abcde"), m.Not(m.Equal(c1a.Key()))))

	c2a := fAnon.NextUniqueContext()
	assert.True(t, c2a.Transient())
	m.In(t).Assert(c2a.Key(), m.AllOf(m.StringHasPrefix("abcde"), m.Not(m.Equal(c1a.Key()))))
	c2b := fAnon.NextUniqueContext()
	assert.True(t, c2b.Transient())
	m.In(t).Assert(c2b.Key(), m.AllOf(m.StringHasPrefix("abcde"), m.Not(m.Equal(c2a.Key()))))
}
