package data

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

var contextRandomizer = rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gochecknoglobals,gosec

// ContextFactory is a test data generator that produces ldcontext.Context instances.
type ContextFactory struct {
	description           string
	prefix                string
	keyDisambiguatorValue int64
	counter               int
	factoryFn             func(string) ldcontext.Context
}

// NewContextFactory creates a ContextFactory that produces single-kind Contexts.
//
// Each generated Context will have a unique key that starts with the prefix string. The builderActions,
// if any, will be run against the builder for each Context. If no actions are specified, then it will
// have no properties other than the key, and its kind will be ldcontext.DefaultKind ("user").
func NewContextFactory(prefix string, builderActions ...func(*ldcontext.Builder)) *ContextFactory {
	return &ContextFactory{
		prefix:                prefix,
		keyDisambiguatorValue: contextRandomizer.Int63(),
		factoryFn: func(key string) ldcontext.Context {
			builder := ldcontext.NewBuilder(key)
			for _, ba := range builderActions {
				ba(builder)
			}
			return builder.Build()
		},
	}
}

// NewMultiContextFactory creates a ContextFactory that produces multi-kind Contexts.
//
// Each generated multiple context will contain an individual context for each of the kinds in the kinds
// parameter. If builderActions are specified, the first one will be run for the first kind, the second
// for the second, etc. Each individual context will have a unique key that starts with the prefix string.
func NewMultiContextFactory(
	prefix string,
	kinds []ldcontext.Kind,
	builderActions ...func(*ldcontext.Builder),
) *ContextFactory {
	return &ContextFactory{
		prefix:                prefix,
		keyDisambiguatorValue: contextRandomizer.Int63(),
		factoryFn: func(key string) ldcontext.Context {
			multiBuilder := ldcontext.NewMultiBuilder()
			for i, kind := range kinds {
				builder := ldcontext.NewBuilder(key)
				builder.Kind(kind)
				if i < len(builderActions) {
					builderActions[i](builder)
				}
				multiBuilder.Add(builder.Build())
			}
			return multiBuilder.Build()
		},
	}
}

// Description returns a descriptive string, if this ContextFactory was produced by a method such as
// NewContextFactoriesForSingleAndMultiKind that provides one.
func (f *ContextFactory) Description() string { return f.description }

func (f *ContextFactory) Prefix() string { return f.prefix }

// NextUniqueContext creates a Context instance.
func (f *ContextFactory) NextUniqueContext() ldcontext.Context {
	f.counter++
	key := fmt.Sprintf("%s.%d.%d", f.prefix, f.keyDisambiguatorValue, f.counter)
	return f.factoryFn(key)
}

// SetKeyDisambiguatorValueSameAs overrides the usual "add a randomized value to all the keys produced
// by this factory" logic, which is meant to avoid key collisions, so that these two factories will
// use the *same* randomized value. This is for tests where we want to verify, for instance, that
// two contexts with the same key but different kinds are treated as distinct.
func (f *ContextFactory) SetKeyDisambiguatorValueSameAs(f1 *ContextFactory) {
	f.keyDisambiguatorValue = f1.keyDisambiguatorValue
}

// NewContextFactoriesForSingleAndMultiKind produces a list of ContextFactory instances for testing SDK
// functionality that may behave differently for different Context variants.
//
// The returned list will include factories for 1. single-kind Contexts of the default kind, 2. single-kind
// Contexts of a different kind, 3. multi-kind Contexts. The reason for checking single vs. multi-kind is
// that we want to make sure the SDK correctly enumerates the kinds when it populates the contextKeys
// property in event data. The reason for checking the default kind vs. a non-default kind is that it
// affects the deduplication logic for index events.
//
// Each will have an appropriate Description, so the logic for running a test against each one can look
// like this:
//
//	for _, contexts := range data.NewContextFactoriesForSingleAndMultiKind("NameOfTest") {
//	    t.Run(contexts.Description(), func(t *testing.T) {
//	        context := contexts.NextUniqueContext() // do something with this
//	    })
//	}
func NewContextFactoriesForSingleAndMultiKind(
	prefix string, builderActions ...func(*ldcontext.Builder),
) []*ContextFactory {
	f1 := NewContextFactory(prefix, builderActions...)
	f1.description = "single kind default"
	f2 := NewContextFactory(prefix, append(builderActions, func(b *ldcontext.Builder) {
		b.Kind("org")
	})...)
	f2.description = "single kind non-default"
	f3 := NewMultiContextFactory(prefix, []ldcontext.Kind{"org", "other"}, builderActions...)
	f3.description = "multi-kind"
	return []*ContextFactory{f1, f2, f3}
}

// NewContextFactoriesForExercisingAllAttributes produces a list of ContextFactory instances that use
// different subsets of the context schema, to verify that context attributes are encoded correctly
// when they are sent to evaluation endpoints.
//
// The returned list will include factories for 1. single-kind Contexts with no attributes other than
// kind and key, 2. single-kind Contexts with every possible kind of attribute, 3. multi-kind Contexts
// that combine 1 and 2.
//
// Each will have an appropriate Description, so the logic for running a test against each one can look
// like this:
//
//	for _, contexts := range data.NewContextFactoriesForExercisingAllAttributes("NameOfTest") {
//	    t.Run(contexts.Description(), func(t *testing.T) {
//	        context := contexts.NextUniqueContext() // do something with this
//	    })
//	}
func NewContextFactoriesForExercisingAllAttributes(
	prefix string,
) []*ContextFactory {
	setAllAttributes := func(b *ldcontext.Builder) {
		b.Name("a").
			Anonymous(true).
			SetBool("c", true).
			SetInt("d", 1).
			SetFloat64("e", 1.5).
			SetString("f", "g").
			SetValue("h", ldvalue.ArrayOf(ldvalue.Int(2))).
			SetValue("i", ldvalue.ObjectBuild().SetBool("j", true).Build())
		b.Private("a", "c")
	}

	f1 := NewContextFactory(prefix)
	f1.description = "single kind minimal"

	f2 := NewContextFactory(prefix, setAllAttributes)
	f2.description = "single kind with all attributes"

	f3 := NewMultiContextFactory(prefix, []ldcontext.Kind{"org", "other"},
		func(b *ldcontext.Builder) {
			b.Name("x")
		},
		setAllAttributes,
	)
	f3.description = "multi-kind"
	return []*ContextFactory{f1, f2, f3}
}
