package data

import (
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

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
	m.In(t).Assert(c2.Key(), m.StringHasPrefix("abcde."))
	m.In(t).Assert(c2.Name(), equalsOptString("x"))
	m.In(t).Assert(c2.Secondary(), equalsOptString("y"))
}

func TestContextFactoryKeyCollisions(t *testing.T) {
	f1, f2 := NewContextFactory("abcde"), NewContextFactory("abcde")
	c1, c2 := f1.NextUniqueContext(), f2.NextUniqueContext()
	assert.NotEqual(t, c1.Key(), c2.Key())

	f3, f4, f5 := NewContextFactory("fghij"), NewContextFactory("fghij"),
		NewMultiContextFactory("fghij", []ldcontext.Kind{"org", "other"})
	f4.SetKeyDisambiguatorValueSameAs(f3)
	f5.SetKeyDisambiguatorValueSameAs(f3)
	c3, c4, c5 := f3.NextUniqueContext(), f4.NextUniqueContext(), f5.NextUniqueContext()
	assert.Equal(t, c3.Key(), c4.Key())
	c5a, _ := c5.MultiKindByIndex(0)
	assert.Equal(t, c3.Key(), c5a.Key())
	c5b, _ := c5.MultiKindByIndex(1)
	assert.Equal(t, c3.Key(), c5b.Key())
}

func TestMultiContextFactory(t *testing.T) {
	f := NewMultiContextFactory("abcde", []ldcontext.Kind{"org", "other"},
		setContextName("x"),      // for MultiContextFactory, the first of these builderActions is only for the first kind
		setContextSecondary("y"), // and the second is only for the second kind
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
	m.In(t).Assert(c2a.Key(), m.StringHasPrefix("abcde."))
	m.In(t).Assert(c2a.Name(), equalsOptString("x"))
	m.In(t).Assert(c2a.Secondary(), optStringEmpty())
	c2b, _ := c2.MultiKindByIndex(1)
	m.In(t).Assert(c2b.Kind(), equalsKind("other"))
	m.In(t).Assert(c2b.Key(), m.StringHasPrefix("abcde."))
	m.In(t).Assert(c2b.Name(), optStringEmpty())
	m.In(t).Assert(c2b.Secondary(), equalsOptString("y"))
}

func TestNewContextFactoriesForSingleAndMultiKind(t *testing.T) {
	fs := NewContextFactoriesForSingleAndMultiKind("abcde",
		setContextName("x"),
		setContextSecondary("y"),
	)
	require.Len(t, fs, 3)

	hasSingleDefault, hasSingleNonDefault, hasMulti := false, false, false
	for _, f := range fs {
		assert.NotEqual(t, "", f.Description())
		c := f.NextUniqueContext()
		if c.Multiple() {
			hasMulti = true
			for i := 0; i < c.MultiKindCount(); i++ {
				mc, _ := c.MultiKindByIndex(i)
				m.In(t).Assert(mc.Key(), m.StringHasPrefix("abcde"))
				if i == 0 {
					m.In(t).Assert(mc.Name(), equalsOptString("x"))
					m.In(t).Assert(mc.Secondary(), optStringEmpty())
				} else {
					m.In(t).Assert(mc.Name(), optStringEmpty())
					m.In(t).Assert(mc.Secondary(), equalsOptString("y"))
				}
			}
		} else {
			if c.Kind() == ldcontext.DefaultKind {
				hasSingleDefault = true
			} else {
				hasSingleNonDefault = true
			}
			m.In(t).Assert(c.Key(), m.StringHasPrefix("abcde"))
			m.In(t).Assert(c.Name(), equalsOptString("x"))
		}
	}
	assert.True(t, hasSingleDefault)
	assert.True(t, hasSingleNonDefault)
	assert.True(t, hasMulti)
}
