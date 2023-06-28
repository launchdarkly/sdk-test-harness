package sdktests

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

func (c CommonEventTests) AutoEnvAttributesNoCollisions(t *ldtest.T) {

	dataSource := NewSDKDataSource(t, nil)
	contextFactories := data.NewContextFactoriesForSingleAndMultiKind(c.contextFactory.Prefix())

	t.Run("opted in", func(t *ldtest.T) {
		for _, contexts := range contextFactories {
			t.Run(contexts.Description(), func(t *ldtest.T) {
				events := NewSDKEventSink(t)
				client := NewSDKClient(t, c.baseSDKConfigurationPlus(
					WithClientSideConfig(servicedef.SDKConfigClientSideParams{IncludeEnvironmentAttributes: opt.Some(true)}),
					dataSource,
					events)...)

				context := contexts.NextUniqueContext()
				client.SendIdentifyEvent(t, context)
				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.Items(
					append(
						c.initialEventPayloadExpectations(),
						m.AllOf(
							m.JSONProperty("context").Should(m.AllOf(
								m.JSONProperty("ld_application").Should(m.AllOf(
									m.JSONProperty("key").Should(m.Not(m.BeNil())),
									m.JSONProperty("envAttributesVersion").Should(m.Not(m.BeNil())),
								)),
								m.JSONProperty("ld_device").Should(m.AllOf(
									m.JSONProperty("key").Should(m.Not(m.BeNil())),
									m.JSONProperty("envAttributesVersion").Should(m.Not(m.BeNil())),
								)),
							)),
						),
					)...,
				))
			})
		}
	})

	t.Run("opted out", func(t *ldtest.T) {
		for _, contexts := range contextFactories {
			t.Run(contexts.Description(), func(t *ldtest.T) {
				events := NewSDKEventSink(t)
				client := NewSDKClient(t, c.baseSDKConfigurationPlus(
					WithClientSideConfig(servicedef.SDKConfigClientSideParams{IncludeEnvironmentAttributes: opt.Some(false)}),
					dataSource,
					events)...)

				context := contexts.NextUniqueContext()
				client.SendIdentifyEvent(t, context)
				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.Items(
					append(
						c.initialEventPayloadExpectations(),
						m.AllOf(
							m.JSONProperty("context").Should(
								m.JSONOptProperty("ld_application").Should(m.BeNil()),
							),
						),
					)...,
				))
			})
		}
	})
}

func (c CommonEventTests) AutoEnvAttributesCollisions(t *ldtest.T) {

	dataSource := NewSDKDataSource(t, nil)

	f1 := data.NewContextFactory(c.contextFactory.Prefix(), func(b *ldcontext.Builder) { b.Kind("ld_application") })
	f2 := data.NewMultiContextFactory(c.contextFactory.Prefix(), []ldcontext.Kind{"ld_application", "other"})
	contextFactories := []*data.ContextFactory{f1, f2}

	t.Run("does not overwrite", func(t *ldtest.T) {
		for _, contexts := range contextFactories {
			t.Run(contexts.Description(), func(t *ldtest.T) {
				events := NewSDKEventSink(t)
				client := NewSDKClient(t, c.baseSDKConfigurationPlus(
					WithClientSideConfig(servicedef.SDKConfigClientSideParams{IncludeEnvironmentAttributes: opt.Some(true)}),
					dataSource,
					events)...)

				context := contexts.NextUniqueContext()

				client.SendIdentifyEvent(t, context)
				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.Items(
					append(
						c.initialEventPayloadExpectations(),
						m.JSONProperty("context").Should(m.AllOf(
							m.JSONProperty("ld_application").Should(
								JSONPropertyNullOrAbsent("envAttributesVersion"),
							),
							m.JSONProperty("ld_device").Should(m.Not(m.BeNil())),
						)),
					)...,
				))
			})
		}
	})
}
