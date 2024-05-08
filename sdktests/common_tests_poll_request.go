package sdktests

import (
	"fmt"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/sdk-test-harness/v2/data"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

func (c CommonPollingTests) RequestMethodAndHeaders(t *ldtest.T, credential string) {
	t.Run("method and headers", func(t *ldtest.T) {
		for _, method := range c.availableFlagRequestMethods() {
			t.Run(string(method), func(t *ldtest.T) {
				for _, transport := range c.availableTransports(t) {
					transport.Run(t, func(t *ldtest.T) {
						dataSource := NewSDKDataSource(t, nil, DataSourceOptionPolling())
						_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
							c.withFlagRequestMethod(method),
							dataSource,
							transport.configurer)...)

						request := dataSource.Endpoint().RequireConnection(t, time.Second)
						m.In(t).For("request method").Assert(request.Method, m.Equal(string(method)))
						m.In(t).For("request headers").Assert(request.Headers, c.authorizationHeaderMatcher(credential))
						if t.Capabilities().Has(servicedef.CapabilityPollingGzip) {
							m.In(t).For("request headers").Assert(request.Headers,
								Header("Accept-Encoding").Should(m.StringContains("gzip")))
						}
					})
				}
			})
		}
	})
	t.Run("invalid tls certificate", func(t *ldtest.T) {
		for _, transport := range c.httpsTransport(t) {
			transport.Run(t, func(t *ldtest.T) {
				dataSource := NewSDKDataSource(t, nil, DataSourceOptionPolling())

				_ = NewSDKClient(t, c.baseSDKConfigurationPlus(dataSource,
					c.withVerifyPeer(true))...)

				_, err := dataSource.Endpoint().AwaitConnection(time.Second)
				assert.Errorf(t, err, "expected connection error")
			})
		}
	})
}

func (c CommonPollingTests) LargePayloads(t *ldtest.T) {
	flags := make([]ldmodel.FeatureFlag, 1000)
	for i := 0; i < 1000; i++ {
		flag := ldmodel.FeatureFlag{
			Key:          fmt.Sprintf("flag-key-%d", i),
			On:           i == 999,
			Variations:   []ldvalue.Value{ldvalue.String("fallthrough"), ldvalue.String("off"), ldvalue.String("default")},
			OffVariation: ldvalue.NewOptionalInt(1),
			Fallthrough: ldmodel.VariationOrRollout{
				Variation: ldvalue.NewOptionalInt(0),
			},
		}
		flags = append(flags, flag)
	}

	sdkData := mockld.NewServerSDKDataBuilder().Flag(flags...).Build()
	dataSource := NewSDKDataSource(t, sdkData, DataSourceOptionPolling())

	t.Run("large payloads", func(t *ldtest.T) {
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSource)...)

		dataSource.Endpoint().RequireConnection(t, time.Second)

		resp := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
			FlagKey:      "flag-key-0",
			ValueType:    servicedef.ValueTypeString,
			Context:      o.Some(ldcontext.New("user-key")),
			DefaultValue: ldvalue.String("default"),
		})

		if !m.In(t).Assert(ldvalue.String("off"), m.JSONEqual(resp.Value)) {
			require.Fail(t, "evaluation unexpectedly returned wrong value")
		}

		resp = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
			FlagKey:      "flag-key-999",
			ValueType:    servicedef.ValueTypeString,
			Context:      o.Some(ldcontext.New("user-key")),
			DefaultValue: ldvalue.String("default"),
		})

		if !m.In(t).Assert(ldvalue.String("fallthrough"), m.JSONEqual(resp.Value)) {
			require.Fail(t, "evaluation unexpectedly returned wrong value")
		}
	})
}

func (c CommonPollingTests) RequestURLPath(t *ldtest.T, pathMatcher func(flagRequestMethod) m.Matcher) {
	t.Run("URL path is computed correctly", func(t *ldtest.T) {
		for _, filter := range c.environmentFilters() {
			t.Run(h.IfElse(filter.IsDefined(), filter.String(), "no environment filter"), func(t *ldtest.T) {
				// The environment filtering feature is only tested on server-side SDKs that support
				// the "filtering" capability. All other SDKs should be tested against the
				// "no filter" scenario (!filter.IsDefined()), since that was the default functionality
				// previous to the introduction of filtering.
				if filter.IsDefined() {
					t.RequireCapability(servicedef.CapabilityFiltering)
					t.RequireCapability(servicedef.CapabilityServerSide)
				}
				for _, trailingSlash := range []bool{false, true} {
					t.Run(h.IfElse(trailingSlash, "base URI has a trailing slash",
						"base URI has no trailing slash"), func(t *ldtest.T) {
						for _, method := range c.availableFlagRequestMethods() {
							t.Run(string(method), func(t *ldtest.T) {
								dataSource := NewSDKDataSource(t, nil, DataSourceOptionPolling())

								pollURI := strings.TrimSuffix(dataSource.Endpoint().BaseURL(), "/")
								if trailingSlash {
									pollURI += "/"
								}

								_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
									c.withFlagRequestMethod(method),
									WithPollingConfig(servicedef.SDKConfigPollingParams{
										BaseURI: pollURI,
										Filter:  filter.Maybe,
									}),
								)...)

								request := dataSource.Endpoint().RequireConnection(t, time.Second)
								m.In(t).For("request path").Assert(request.URL.Path, pathMatcher(method))
								m.In(t).For("filter key").Assert(request.URL.RawQuery, filter.Matcher())
							})
						}
					})
				}
			})
		}
	})

	if c.isClientSide {
		t.Run("query parameters", func(t *ldtest.T) {
			for _, withReasons := range []o.Maybe[bool]{o.None[bool](), o.Some(false), o.Some(true)} {
				// The reason we use 3 states here instead of 2 is to verify that the SDK uses a default
				// of false if we *don't* set the property.

				t.Run(fmt.Sprintf("evaluationReasons set to %s", withReasons), func(t *ldtest.T) {
					for _, method := range c.availableFlagRequestMethods() {
						t.Run(string(method), func(t *ldtest.T) {
							dataSource := NewSDKDataSource(t, nil, DataSourceOptionPolling())

							_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
								c.withFlagRequestMethod(method),
								WithClientSideConfig(servicedef.SDKConfigClientSideParams{
									EvaluationReasons: withReasons,
									InitialContext:    o.Some(ldcontext.New("irrelevant-key")),
								}),
								dataSource,
							)...)

							request := dataSource.Endpoint().RequireConnection(t, time.Second)

							var queryMatcher m.Matcher
							if withReasons.Value() {
								queryMatcher = m.MapOf(
									m.KV("withReasons", m.Items(m.Equal("true"))),
								)
							} else {
								queryMatcher = m.AnyOf(
									m.MapOf(
										m.KV("withReasons", m.Items(m.Equal("false"))),
									),
									m.MapOf(),
								)
							}
							m.In(t).For("query string").Assert(request.URL.Query(), queryMatcher)
						})
					}
				})
			}
		})
	}
}

func (c CommonPollingTests) RequestContextProperties(t *ldtest.T, getPath string) {
	t.RequireCapability(servicedef.CapabilityClientSide) // server-side SDKs do not send user properties in stream requests

	t.Run("context properties", func(t *ldtest.T) {
		for _, contexts := range data.NewContextFactoriesForExercisingAllAttributes(c.contextFactory.Prefix()) {
			t.Run(contexts.Description(), func(t *ldtest.T) {
				for _, method := range c.availableFlagRequestMethods() {
					t.Run(string(method), func(t *ldtest.T) {
						dataSource := NewSDKDataSource(t, nil, DataSourceOptionPolling())

						context := contexts.NextUniqueContext()
						contextJSONMatcher := JSONMatchesContext(context)

						_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
							WithClientSideInitialContext(context),
							c.withFlagRequestMethod(method),
							dataSource,
						)...)

						request := dataSource.Endpoint().RequireConnection(t, time.Second)

						if method == flagRequestREPORT {
							m.In(t).For("request body").Assert(request.Body, m.AllOf(
								m.Not(m.BeNil()),
								contextJSONMatcher))
						} else {
							m.In(t).For("request body").Assert(request.Body, m.Length().Should(m.Equal(0)))

							getPathPrefix := strings.TrimSuffix(getPath, mockld.StreamingPathContextBase64Param)
							m.In(t).For("request path").Require(request.URL.Path, m.StringHasPrefix(getPathPrefix))
							contextData := strings.TrimPrefix(request.URL.Path, getPathPrefix)

							m.In(t).For("context data in URL").Assert(contextData,
								Base64DecodedData().Should(contextJSONMatcher))
						}
					})
				}
			})
		}
	})
}

func (c CommonPollingTests) InitialRequestIncludesCorrectEtag(t *ldtest.T) {
	contexts := data.NewContextFactory("etag-header")

	t.Run("e-tag", func(t *ldtest.T) {
		t.Run("is not set on initial request", func(t *ldtest.T) {
			for _, method := range c.availableFlagRequestMethods() {
				context := contexts.NextUniqueContext()

				dataSource := NewSDKDataSource(t, nil, DataSourceOptionPolling())
				dataSource.pollingService.SetEtag(context.FullyQualifiedKey())

				client := NewSDKClient(t, c.baseSDKConfigurationPlus(
					WithClientSideInitialContext(context),
					c.withFlagRequestMethod(method),
					dataSource,
				)...)

				request := dataSource.Endpoint().RequireConnection(t, time.Second)
				m.In(t).For("request headers").Assert(request.Headers, Header("If-None-Match").Should(m.Equal("")))

				_ = client.Close()

				dataSource = NewSDKDataSource(t, nil, DataSourceOptionPolling())
				client = NewSDKClient(t, c.baseSDKConfigurationPlus(
					WithClientSideInitialContext(context),
					c.withFlagRequestMethod(method),
					dataSource,
				)...)

				request = dataSource.Endpoint().RequireConnection(t, time.Second)
				m.In(t).For("request headers").Assert(
					request.Headers,
					Header("If-None-Match").Should(m.Equal(context.FullyQualifiedKey())),
				)
				_ = client.Close()
			}
		})

		t.Run("is different for different contexts", func(t *ldtest.T) {
			for _, method := range c.availableFlagRequestMethods() {
				context1 := contexts.NextUniqueContext()
				context2 := contexts.NextUniqueContext()
				contexts := []ldcontext.Context{context1, context2}

				for _, context := range contexts {
					// Initialize and close clients with multiple contexts. Each one should use a different e-tag value
					dataSource := NewSDKDataSource(t, nil, DataSourceOptionPolling())
					dataSource.pollingService.SetEtag(context.FullyQualifiedKey())
					client := NewSDKClient(t, c.baseSDKConfigurationPlus(
						WithClientSideInitialContext(context),
						c.withFlagRequestMethod(method),
						dataSource,
					)...)

					request := dataSource.Endpoint().RequireConnection(t, time.Second)
					m.In(t).For("request headers").Assert(request.Headers, Header("If-None-Match").Should(m.Equal("")))

					_ = client.Close()
				}

				// Then re-initialize each context, verifying the e-tag is right for each.
				for _, context := range contexts {
					dataSource := NewSDKDataSource(t, nil, DataSourceOptionPolling())
					client := NewSDKClient(t, c.baseSDKConfigurationPlus(
						WithClientSideInitialContext(context),
						c.withFlagRequestMethod(method),
						dataSource,
					)...)

					request := dataSource.Endpoint().RequireConnection(t, time.Second)
					m.In(t).For("request headers").Assert(
						request.Headers,
						Header("If-None-Match").Should(m.Equal(context.FullyQualifiedKey())),
					)

					_ = client.Close()
				}
			}
		})

		t.Run("considers the full context hash", func(t *ldtest.T) {
			for _, method := range c.availableFlagRequestMethods() {
				context1 := contexts.NextUniqueContext()

				// These attributes would affect a full context hash, but not the fully qualified key.
				builder := ldcontext.NewBuilderFromContext(context1)
				builder.Name("context 2")
				builder.SetInt("age", 42)
				builder.SetString("favorite color", "purple")
				context2 := builder.Build()

				m.In(t).Assert(context1.FullyQualifiedKey(), m.Equal(context2.FullyQualifiedKey()))

				// Initialize and close clients with multiple contexts. Each one should use a different e-tag value
				dataSource := NewSDKDataSource(t, nil, DataSourceOptionPolling())
				dataSource.pollingService.SetEtag(context1.FullyQualifiedKey())
				client := NewSDKClient(t, c.baseSDKConfigurationPlus(
					WithClientSideInitialContext(context1),
					c.withFlagRequestMethod(method),
					dataSource,
				)...)

				request := dataSource.Endpoint().RequireConnection(t, time.Second)
				m.In(t).For("request headers").Assert(request.Headers, Header("If-None-Match").Should(m.Equal("")))

				_ = client.Close()

				dataSource = NewSDKDataSource(t, nil, DataSourceOptionPolling())
				client = NewSDKClient(t, c.baseSDKConfigurationPlus(
					WithClientSideInitialContext(context2),
					c.withFlagRequestMethod(method),
					dataSource,
				)...)

				request = dataSource.Endpoint().RequireConnection(t, time.Second)
				m.In(t).For("request headers").Assert(request.Headers, Header("If-None-Match").Should(m.Equal("")))

				_ = client.Close()
			}
		})

		t.Run("is not reset if streaming is used", func(t *ldtest.T) {
			for _, method := range c.availableFlagRequestMethods() {
				context := contexts.NextUniqueContext()

				// Setup an initial polling request with a defined e-tag value
				dataSource := NewSDKDataSource(t, nil, DataSourceOptionPolling())
				dataSource.pollingService.SetEtag(context.FullyQualifiedKey())

				client := NewSDKClient(t, c.baseSDKConfigurationPlus(
					WithClientSideInitialContext(context),
					c.withFlagRequestMethod(method),
					dataSource,
				)...)

				request := dataSource.Endpoint().RequireConnection(t, time.Second)
				m.In(t).For("request headers").Assert(request.Headers, Header("If-None-Match").Should(m.Equal("")))

				_ = client.Close()

				// Initializing a new instance with a streaming mode connection. This should not affect the cached e-tag
				dataSource = NewSDKDataSource(t, nil, DataSourceOptionStreaming())
				client = NewSDKClient(t, c.baseSDKConfigurationPlus(
					WithClientSideInitialContext(context),
					c.withFlagRequestMethod(method),
					dataSource,
				)...)

				request = dataSource.Endpoint().RequireConnection(t, time.Second)
				_ = client.Close()

				// So setup another polling client and make sure the e-tag value is blank.
				dataSource = NewSDKDataSource(t, nil, DataSourceOptionPolling())
				client = NewSDKClient(t, c.baseSDKConfigurationPlus(
					WithClientSideInitialContext(context),
					c.withFlagRequestMethod(method),
					dataSource,
				)...)

				request = dataSource.Endpoint().RequireConnection(t, time.Second)
				m.In(t).For("request headers").Assert(
					request.Headers,
					Header("If-None-Match").Should(m.Equal(context.FullyQualifiedKey())),
				)
				_ = client.Close()
			}
		})
	})
}
