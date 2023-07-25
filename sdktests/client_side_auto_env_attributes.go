package sdktests

import (
	"strings"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
	"github.com/stretchr/testify/assert"
)

func doClientSideAutoEnvAttributesTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityAutoEnvAttributes)
	t.Run("events", doClientSideAutoEnvAttributesEventsTests)
	t.Run("headers", doClientSideAutoEnvAttributesHeaderTests)

	// server-side SDKs do not send user properties in polling/streaming requests
	if t.Capabilities().Has(servicedef.CapabilityClientSide) {
		t.Run("pollingAndStreaming", doClientSideAutoEnvAttributesRequestingTests)
	}
}

func doClientSideAutoEnvAttributesEventsTests(t *ldtest.T) {
	t.Run("no collisions", doClientSideAutoEnvAttributesEventsNoCollisionsTests)
	t.Run("collisions", doClientSideAutoEnvAttributesEventsCollisionsTests)
}

func doClientSideAutoEnvAttributesRequestingTests(t *ldtest.T) {
	t.Run("no collisions", doClientSideAutoEnvAttributesRequestingNoCollisionsTests)
	t.Run("collisions", doClientSideAutoEnvAttributesRequestingCollisionsTests)
}

// Start tests for events
func doClientSideAutoEnvAttributesEventsNoCollisionsTests(t *ldtest.T) {
	base := newCommonTestsBase(t, "doClientSideAutoEnvAttributesEventsNoCollisionsTests")
	dataSource := NewSDKDataSource(t, nil)
	contextFactories := data.NewContextFactoriesForSingleAndMultiKind(base.contextFactory.Prefix())

	t.Run("opted in", func(t *ldtest.T) {
		for _, contexts := range contextFactories {
			t.Run(contexts.Description(), func(t *ldtest.T) {
				events := NewSDKEventSink(t)
				client := NewSDKClient(t, base.baseSDKConfigurationPlus(
					WithClientSideConfig(servicedef.SDKConfigClientSideParams{IncludeEnvironmentAttributes: opt.Some(true)}),
					dataSource,
					events)...)

				context := contexts.NextUniqueContext()
				client.SendIdentifyEvent(t, context)
				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.Items(
					append(
						[]m.Matcher{IsIdentifyEvent()},
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
				client := NewSDKClient(t, base.baseSDKConfigurationPlus(
					WithClientSideConfig(servicedef.SDKConfigClientSideParams{IncludeEnvironmentAttributes: opt.Some(false)}),
					dataSource,
					events)...)

				context := contexts.NextUniqueContext()
				client.SendIdentifyEvent(t, context)
				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

				context_matchers := []m.Matcher{
					m.JSONOptProperty("ld_application").Should(m.BeNil()),
					m.JSONOptProperty("ld_device").Should(m.BeNil()),
				}

				if context.Multiple() {
					for _, c := range context.GetAllIndividualContexts(nil) {
						context_matchers = append(context_matchers, m.JSONProperty(string(c.Kind())).Should(m.Not(m.BeNil())))
					}
				} else {
					context_matchers = append(context_matchers, m.JSONProperty("kind").Should(m.Equal(string(context.Kind()))))
				}

				m.In(t).Assert(payload, m.Items(
					append(
						[]m.Matcher{IsIdentifyEvent()},
						m.JSONProperty("context").Should(m.AllOf(context_matchers...)),
					)...,
				))
			})
		}
	})
}

func doClientSideAutoEnvAttributesEventsCollisionsTests(t *ldtest.T) {
	base := newCommonTestsBase(t, "doClientSideAutoEnvAttributesEventsCollisionsTests")
	dataSource := NewSDKDataSource(t, nil)

	f1 := data.NewContextFactory(base.contextFactory.Prefix(), func(b *ldcontext.Builder) { b.Kind("ld_application") })
	f2 := data.NewMultiContextFactory(base.contextFactory.Prefix(), []ldcontext.Kind{"ld_application", "other"})
	contextFactories := []*data.ContextFactory{f1, f2}

	t.Run("does not overwrite", func(t *ldtest.T) {
		for _, contexts := range contextFactories {
			t.Run(contexts.Description(), func(t *ldtest.T) {
				events := NewSDKEventSink(t)
				client := NewSDKClient(t, base.baseSDKConfigurationPlus(
					WithClientSideConfig(servicedef.SDKConfigClientSideParams{IncludeEnvironmentAttributes: opt.Some(true)}),
					dataSource,
					events)...)

				context := contexts.NextUniqueContext()

				client.SendIdentifyEvent(t, context)
				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.Items(
					append(
						[]m.Matcher{IsIdentifyEvent()},
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

// end tests for events

// start tests for streaming/polling
func doClientSideAutoEnvAttributesRequestingNoCollisionsTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityClientSide) // server-side SDKs do not send user properties in stream requests
	base := newCommonTestsBase(t, "doClientSideAutoEnvAttributesPollNoCollisionsTests")
	dsos := []SDKDataSourceOption{DataSourceOptionPolling(), DataSourceOptionStreaming()}
	for _, dso := range dsos {
		contextFactories := data.NewContextFactoriesForSingleAndMultiKind(base.contextFactory.Prefix())
		for _, contexts := range contextFactories {
			t.Run(contexts.Description(), func(t *ldtest.T) {
				// for _, method := range base.availableFlagRequestMethods() {
				dataSource := NewSDKDataSource(t, nil, dso)
				context := contexts.NextUniqueContext()

				_ = NewSDKClient(t, base.baseSDKConfigurationPlus(
					WithClientSideConfig(servicedef.SDKConfigClientSideParams{IncludeEnvironmentAttributes: opt.Some(true)}),
					WithClientSideInitialContext(context),
					base.withFlagRequestMethod(flagRequestREPORT),
					dataSource,
				)...)

				request := dataSource.Endpoint().RequireConnection(t, time.Second)

				m.In(t).For("request body").Assert(request.Body, m.AllOf(
					m.JSONProperty("ld_application").Should(m.AllOf(
						m.JSONProperty("key").Should(m.Not(m.BeNil())),
						m.JSONProperty("envAttributesVersion").Should(m.Not(m.BeNil())),
					)),
					m.JSONProperty("ld_device").Should(m.AllOf(
						m.JSONProperty("key").Should(m.Not(m.BeNil())),
						m.JSONProperty("envAttributesVersion").Should(m.Not(m.BeNil())),
					)),
				))
			})
		}
	}
}

func doClientSideAutoEnvAttributesRequestingCollisionsTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityClientSide) // server-side SDKs do not send user properties in stream requests
	base := newCommonTestsBase(t, "doClientSideAutoEnvAttributesPollNoCollisionsTests")
	dsos := []SDKDataSourceOption{DataSourceOptionPolling(), DataSourceOptionStreaming()}
	for _, dso := range dsos {
		f1 := data.NewContextFactory(base.contextFactory.Prefix(), func(b *ldcontext.Builder) { b.Kind("ld_application") })
		f2 := data.NewMultiContextFactory(base.contextFactory.Prefix(), []ldcontext.Kind{"ld_application", "other"})
		contextFactories := []*data.ContextFactory{f1, f2}
		for _, contexts := range contextFactories {
			t.Run(contexts.Description(), func(t *ldtest.T) {
				// for _, method := range base.availableFlagRequestMethods() {
				dataSource := NewSDKDataSource(t, nil, dso)
				context := contexts.NextUniqueContext()

				_ = NewSDKClient(t, base.baseSDKConfigurationPlus(
					WithClientSideConfig(servicedef.SDKConfigClientSideParams{IncludeEnvironmentAttributes: opt.Some(true)}),
					WithClientSideInitialContext(context),
					base.withFlagRequestMethod(flagRequestREPORT),
					dataSource,
				)...)

				request := dataSource.Endpoint().RequireConnection(t, time.Second)

				m.In(t).For("request body").Assert(request.Body, m.AllOf(
					m.JSONProperty("ld_application").Should(
						JSONPropertyNullOrAbsent("envAttributesVersion"),
					),
					m.JSONProperty("ld_device").Should(m.Not(m.BeNil())),
				))
			})
		}
	}
}

// end tests for streaming/polling

// start tests for headers
func doClientSideAutoEnvAttributesHeaderTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityTags)
	base := newCommonTestsBase(t, "doClientSideAutoEnvAttributesEventsCollisionsTests")

	verifyRequestHeader := func(t *ldtest.T, endpoint *harness.MockEndpoint) {
		request := endpoint.RequireConnection(t, time.Second)

		header := request.Headers.Get("X-LaunchDarkly-Tags")
		assert.NotEmpty(t, header)

		// Deconstruct header into name/value pairs
		nameValuePairs := make(map[string]string)
		for _, pair := range strings.Split(header, " ") {
			parts := strings.Split(pair, "/")
			assert.Len(t, parts, 2)

			nameValuePairs[parts[0]] = parts[1]
		}

		for _, expectedTag := range []string{"application-id", "application-version", "application-version-name"} {
			value, found := nameValuePairs[expectedTag]
			assert.True(t, found, "Provided tags did not contain %s", expectedTag)
			assert.NotEmpty(t, value, "Value for tag %s is empty", expectedTag)
		}
	}

	t.Run("stream requests", func(t *ldtest.T) {
		dataSource := NewSDKDataSource(t, nil, DataSourceOptionStreaming())
		configurers := base.baseSDKConfigurationPlus(
			WithClientSideConfig(servicedef.SDKConfigClientSideParams{IncludeEnvironmentAttributes: opt.Some(true)}),
			dataSource)

		if base.isClientSide {
			// client-side SDKs in streaming mode may *also* need a polling data source
			configurers = append(configurers,
				NewSDKDataSource(t, nil, DataSourceOptionPolling()))
		}
		_ = NewSDKClient(t, configurers...)
		verifyRequestHeader(t, dataSource.Endpoint())
	})

	t.Run("poll requests", func(t *ldtest.T) {
		// Currently server-side SDK test services do not support polling
		t.RequireCapability(servicedef.CapabilityClientSide)

		dataSource := NewSDKDataSource(t, nil, DataSourceOptionPolling())
		_ = NewSDKClient(t, base.baseSDKConfigurationPlus(
			WithClientSideConfig(servicedef.SDKConfigClientSideParams{IncludeEnvironmentAttributes: opt.Some(true)}),
			dataSource)...)
		verifyRequestHeader(t, dataSource.Endpoint())
	})

	t.Run("event posts", func(t *ldtest.T) {
		dataSource := NewSDKDataSource(t, nil)
		events := NewSDKEventSink(t)
		client := NewSDKClient(t, base.baseSDKConfigurationPlus(
			WithClientSideConfig(servicedef.SDKConfigClientSideParams{IncludeEnvironmentAttributes: opt.Some(true)}),
			dataSource,
			events)...)

		base.sendArbitraryEvent(t, client)
		client.FlushEvents(t)

		verifyRequestHeader(t, events.Endpoint())
	})
}

// end tests for headers
