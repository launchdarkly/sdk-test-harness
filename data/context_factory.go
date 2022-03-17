package data

import (
	"fmt"

	"gopkg.in/launchdarkly/go-sdk-common.v3/ldcontext"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldtime"
)

// ContextFactory is a test data generator that produces ldcontext.Context instances.
type ContextFactory struct {
	description string
	prefix      string
	createdTime ldtime.UnixMillisecondTime
	counter     int
	factoryFn   func(string) ldcontext.Context
}

// NewContextFactory creates a ContextFactory that produces single-kind Contexts.
//
// Each generated Context will have a unique key that starts with the prefix string. The builderActions,
// if any, will be run against the builder for each Context. If no actions are specified, then it will
// have no properties other than the key, and its kind will be ldcontext.DefaultKind ("user").
func NewContextFactory(prefix string, builderActions ...func(*ldcontext.Builder)) *ContextFactory {
	return &ContextFactory{
		prefix:      prefix,
		createdTime: ldtime.UnixMillisNow(),
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
		prefix:      prefix,
		createdTime: ldtime.UnixMillisNow(),
		factoryFn: func(key string) ldcontext.Context {
			multiBuilder := ldcontext.NewMultiBuilder()
			for i, kind := range kinds {
				builder := ldcontext.NewBuilder(key + "." + string(kind))
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

// NextUniqueContext creates a Context instance.
func (f *ContextFactory) NextUniqueContext() ldcontext.Context {
	f.counter++
	key := fmt.Sprintf("%s.%d.%d", f.prefix, f.createdTime, f.counter)
	return f.factoryFn(key)
}

func NewContextFactoriesForAnonymousAndNonAnonymous(
	prefix string,
	builderActions ...func(*ldcontext.Builder),
) []*ContextFactory {
	f1 := NewContextFactory(prefix+".nonanon", builderActions...)
	f1.description = "non-anonymous user"
	f2 := NewContextFactory(prefix+".anon", append(builderActions, func(b *ldcontext.Builder) {
		b.Transient(true)
	})...)
	f2.description = "anonymous user"
	return []*ContextFactory{f1, f2}
}
