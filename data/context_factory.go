package data

import (
	"fmt"

	"gopkg.in/launchdarkly/go-sdk-common.v3/ldcontext"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldtime"
)

type ContextFactory struct {
	description string
	prefix      string
	createdTime ldtime.UnixMillisecondTime
	counter     int
	factoryFn   func(string) ldcontext.Context
}

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

func NewContextFactory(prefix string, builderActions ...func(*ldcontext.Builder)) *ContextFactory {
	return &ContextFactory{
		prefix:      prefix,
		createdTime: ldtime.UnixMillisNow(),
		factoryFn: func(key string) ldcontext.Context {
			builder := ldcontext.NewBuilder(key)
			for _, ba := range builderActions {
				if ba != nil {
					ba(builder)
				}
			}
			return builder.Build()
		},
	}
}

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
			for _, kind := range kinds {
				builder := ldcontext.NewBuilder(key + "." + string(kind))
				builder.Kind(kind)
				for _, ba := range builderActions {
					if ba != nil {
						ba(builder)
					}
				}
				multiBuilder.Add(builder.Build())
			}
			return multiBuilder.Build()
		},
	}
}

func (f *ContextFactory) Description() string { return f.description }

func (f *ContextFactory) NextUniqueContext() ldcontext.Context {
	f.counter++
	key := fmt.Sprintf("%s.%d.%d", f.prefix, f.createdTime, f.counter)
	return f.factoryFn(key)
}
