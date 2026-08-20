package sdktests

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/sdk-test-harness/v3/data"
	h "github.com/launchdarkly/sdk-test-harness/v3/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

func (c CommonPollingTests) RequestMethodAndHeaders(t *ldtest.T, credential string) {
	t.Run("method and headers", func(t *ldtest.T) {
		for _, method := range c.availableFlagRequestMethods(t) {
			t.Run(string(method), func(t *ldtest.T) {
				for _, transport := range c.withAvailableTransports(t) {
					transport.Run(t, func(t *ldtest.T) {
						dataSystem := NewSDKDataSystem(t, nil, c.pollingDataSystemOptions()...)
						_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
							c.withFlagRequestMethod(method),
							dataSystem,
							transport.configurer)...)

						request := dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)
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
		c.withHTTPSTransport(t).Run(t, func(t *ldtest.T) {
			dataSystem := NewSDKDataSystem(t, nil, c.pollingDataSystemOptions()...)

			_ = NewSDKClient(t, c.baseSDKConfigurationPlus(dataSystem)...)

			_, err := dataSystem.Synchronizers[0].Endpoint().AwaitConnection(time.Second)
			assert.Errorf(t, err, "expected connection error")
		})
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
	dataSystem := NewSDKDataSystem(t, sdkData, c.pollingDataSystemOptions()...)

	t.Run("large payloads", func(t *ldtest.T) {
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSystem)...)

		dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)

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
		for _, trailingSlash := range []bool{false, true} {
			t.Run(h.IfElse(trailingSlash, "base URI has a trailing slash",
				"base URI has no trailing slash"), func(t *ldtest.T) {
				for _, method := range c.availableFlagRequestMethods(t) {
					t.Run(string(method), func(t *ldtest.T) {
						dataSystem := NewSDKDataSystem(t, nil, c.pollingDataSystemOptions()...)

						pollURI := strings.TrimSuffix(dataSystem.Synchronizers[0].Endpoint().BaseURL(), "/")
						if trailingSlash {
							pollURI += "/"
						}

						var uriConfigurer SDKConfigurer
						if c.isClientSide {
							uriConfigurer = WithConnectionModeSynchronizer("polling", servicedef.DataSynchronizer{
								Polling: o.Some(servicedef.SDKConfigPollingParams{BaseURI: pollURI}),
							})
						} else {
							uriConfigurer = WithPollingSynchronizer(servicedef.SDKConfigPollingParams{
								BaseURI: pollURI,
							})
						}
						baseConfigurers := []SDKConfigurer{
							c.withFlagRequestMethod(method),
							uriConfigurer,
						}
						if c.isClientSide {
							baseConfigurers = append(baseConfigurers, WithInitialConnectionMode("polling"))
						}
						_ = NewSDKClient(t, c.baseSDKConfigurationPlus(baseConfigurers...)...)

						request := dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)
						m.In(t).For("request path").Assert(request.URL.Path, pathMatcher(method))
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
					for _, method := range c.availableFlagRequestMethods(t) {
						t.Run(string(method), func(t *ldtest.T) {
							dataSystem := NewSDKDataSystem(t, nil, c.pollingDataSystemOptions()...)

							_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
								c.withFlagRequestMethod(method),
								WithClientSideConfig(servicedef.SDKConfigClientSideParams{
									EvaluationReasons: withReasons,
									InitialContext:    o.Some(ldcontext.New("irrelevant-key")),
								}),
								dataSystem,
							)...)

							request := dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)

							withReasonsParam := request.URL.Query().Get("withReasons")
							if withReasons.Value() {
								m.In(t).For("withReasons query parameter").Assert(withReasonsParam, m.Equal("true"))
							} else {
								m.In(t).For("withReasons query parameter").Assert(withReasonsParam,
									m.AnyOf(m.Equal("false"), m.Equal("")))
							}
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
				for _, method := range c.availableFlagRequestMethods(t) {
					t.Run(string(method), func(t *ldtest.T) {
						dataSystem := NewSDKDataSystem(t, nil, c.pollingDataSystemOptions()...)

						context := contexts.NextUniqueContext()
						contextJSONMatcher := JSONMatchesContext(context)

						_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
							WithClientSideInitialContext(context),
							c.withFlagRequestMethod(method),
							dataSystem,
						)...)

						request := dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)

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
			for _, method := range c.availableFlagRequestMethods(t) {
				context := contexts.NextUniqueContext()

				dataSystem := NewSDKDataSystem(t, nil, c.pollingDataSystemOptions()...)
				dataSystem.Synchronizers[0].polling.SetEtag(context.FullyQualifiedKey())

				client := NewSDKClient(t, c.baseSDKConfigurationPlus(
					WithClientSideInitialContext(context),
					c.withFlagRequestMethod(method),
					dataSystem,
				)...)

				request := dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)
				m.In(t).For("request headers").Assert(request.Headers, Header("If-None-Match").Should(m.Equal("")))

				_ = client.Close()

				dataSystem = NewSDKDataSystem(t, nil, c.pollingDataSystemOptions()...)
				client = NewSDKClient(t, c.baseSDKConfigurationPlus(
					WithClientSideInitialContext(context),
					c.withFlagRequestMethod(method),
					dataSystem,
				)...)

				request = dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)
				m.In(t).For("request headers").Assert(
					request.Headers,
					Header("If-None-Match").Should(m.Equal(context.FullyQualifiedKey())),
				)
				_ = client.Close()
			}
		})

		t.Run("is different for different contexts", func(t *ldtest.T) {
			for _, method := range c.availableFlagRequestMethods(t) {
				context1 := contexts.NextUniqueContext()
				context2 := contexts.NextUniqueContext()
				contexts := []ldcontext.Context{context1, context2}

				for _, context := range contexts {
					// Initialize and close clients with multiple contexts. Each one should use a different e-tag value
					dataSystem := NewSDKDataSystem(t, nil, c.pollingDataSystemOptions()...)
					dataSystem.Synchronizers[0].polling.SetEtag(context.FullyQualifiedKey())
					client := NewSDKClient(t, c.baseSDKConfigurationPlus(
						WithClientSideInitialContext(context),
						c.withFlagRequestMethod(method),
						dataSystem,
					)...)

					request := dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)
					m.In(t).For("request headers").Assert(request.Headers, Header("If-None-Match").Should(m.Equal("")))

					_ = client.Close()
				}

				// Then re-initialize each context, verifying the e-tag is right for each.
				for _, context := range contexts {
					dataSystem := NewSDKDataSystem(t, nil, c.pollingDataSystemOptions()...)
					client := NewSDKClient(t, c.baseSDKConfigurationPlus(
						WithClientSideInitialContext(context),
						c.withFlagRequestMethod(method),
						dataSystem,
					)...)

					request := dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)
					m.In(t).For("request headers").Assert(
						request.Headers,
						Header("If-None-Match").Should(m.Equal(context.FullyQualifiedKey())),
					)

					_ = client.Close()
				}
			}
		})

		t.Run("considers the full context hash", func(t *ldtest.T) {
			for _, method := range c.availableFlagRequestMethods(t) {
				context1 := contexts.NextUniqueContext()

				// These attributes would affect a full context hash, but not the fully qualified key.
				builder := ldcontext.NewBuilderFromContext(context1)
				builder.Name("context 2")
				builder.SetInt("age", 42)
				builder.SetString("favorite color", "purple")
				context2 := builder.Build()

				m.In(t).Assert(context1.FullyQualifiedKey(), m.Equal(context2.FullyQualifiedKey()))

				// Initialize and close clients with multiple contexts. Each one should use a different e-tag value
				dataSystem := NewSDKDataSystem(t, nil, c.pollingDataSystemOptions()...)
				dataSystem.Synchronizers[0].polling.SetEtag(context1.FullyQualifiedKey())
				client := NewSDKClient(t, c.baseSDKConfigurationPlus(
					WithClientSideInitialContext(context1),
					c.withFlagRequestMethod(method),
					dataSystem,
				)...)

				request := dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)
				m.In(t).For("request headers").Assert(request.Headers, Header("If-None-Match").Should(m.Equal("")))

				_ = client.Close()

				dataSystem = NewSDKDataSystem(t, nil, c.pollingDataSystemOptions()...)
				client = NewSDKClient(t, c.baseSDKConfigurationPlus(
					WithClientSideInitialContext(context2),
					c.withFlagRequestMethod(method),
					dataSystem,
				)...)

				request = dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)
				m.In(t).For("request headers").Assert(request.Headers, Header("If-None-Match").Should(m.Equal("")))

				_ = client.Close()
			}
		})

		t.Run("is not reset if streaming is used", func(t *ldtest.T) {
			for _, method := range c.availableFlagRequestMethods(t) {
				context := contexts.NextUniqueContext()

				// Setup an initial polling request with a defined e-tag value
				dataSystem := NewSDKDataSystem(t, nil, c.pollingDataSystemOptions()...)
				dataSystem.Synchronizers[0].polling.SetEtag(context.FullyQualifiedKey())

				client := NewSDKClient(t, c.baseSDKConfigurationPlus(
					WithClientSideInitialContext(context),
					c.withFlagRequestMethod(method),
					dataSystem,
				)...)

				request := dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)
				m.In(t).For("request headers").Assert(request.Headers, Header("If-None-Match").Should(m.Equal("")))

				_ = client.Close()

				// Initializing a new instance with a streaming mode connection. This should not affect the cached e-tag
				dataSystem = NewSDKDataSystem(t, nil, c.streamingDataSystemOptions()...)
				client = NewSDKClient(t, c.baseSDKConfigurationPlus(
					WithClientSideInitialContext(context),
					c.withFlagRequestMethod(method),
					dataSystem,
				)...)

				request = dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)
				_ = client.Close()

				// So setup another polling client and make sure the e-tag value is blank.
				dataSystem = NewSDKDataSystem(t, nil, c.pollingDataSystemOptions()...)
				client = NewSDKClient(t, c.baseSDKConfigurationPlus(
					WithClientSideInitialContext(context),
					c.withFlagRequestMethod(method),
					dataSystem,
				)...)

				request = dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)
				m.In(t).For("request headers").Assert(
					request.Headers,
					Header("If-None-Match").Should(m.Equal(context.FullyQualifiedKey())),
				)
				_ = client.Close()
			}
		})
	})
}

func (c CommonPollingTests) RequestViaHTTPProxy(t *ldtest.T) {
	t.Run("http proxy", func(t *ldtest.T) {
		dataSystem := NewSDKDataSystem(t, nil, c.pollingDataSystemOptions()...)

		// The idea here is that we'll configure the SDK's service endpoints with an arbitrary host, but with the
		// correct path that the test harness expects (like /endpoints/1). Then, we'll inject the actual test harness's
		// endpoint via the HTTP Proxy configuration.
		//
		// The SDK should therefore:
		// 1. Open a socket to the test harness's host and port
		// 2. Send an HTTP request that has the arbitrary host and the correct path
		//
		// If the SDK didn't support proxying, then it would attempt to connect to the arbitrary host and
		// the harness should fail the connection assertion.
		pollURI := strings.Replace(dataSystem.Synchronizers[0].Endpoint().BaseURL(), "localhost", "not.valid.local", 1)

		u, err := url.Parse(dataSystem.Synchronizers[0].Endpoint().BaseURL())
		if err != nil {
			t.Errorf("unexpected error parsing URL: %s", err)
			t.FailNow()
		}
		u.Path = ""

		var uriConfigurer SDKConfigurer
		if c.isClientSide {
			uriConfigurer = WithConnectionModeSynchronizer("polling", servicedef.DataSynchronizer{
				Polling: o.Some(servicedef.SDKConfigPollingParams{BaseURI: pollURI}),
			})
		} else {
			uriConfigurer = WithPollingSynchronizer(servicedef.SDKConfigPollingParams{
				BaseURI: pollURI,
			})
		}

		configurers := []SDKConfigurer{uriConfigurer, c.withHTTPProxy(u.String())}
		if c.isClientSide {
			configurers = append(configurers, WithInitialConnectionMode("polling"))
		}

		_ = NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)

		_, err = dataSystem.Synchronizers[0].Endpoint().AwaitConnection(time.Second)
		assert.NoError(t, err)
	})
}
