package sdktests

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/sdk-test-harness/v3/data"
	h "github.com/launchdarkly/sdk-test-harness/v3/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

func (c CommonStreamingTests) RequestMethodAndHeaders(t *ldtest.T, credential string) {
	t.Run("method and headers", func(t *ldtest.T) {
		for _, method := range c.availableFlagRequestMethods(t) {
			t.Run(string(method), func(t *ldtest.T) {
				for _, transport := range c.withAvailableTransports(t) {
					transport.Run(t, func(t *ldtest.T) {
						dataSystem, configurers := c.setupDataSystems(t, nil)

						_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
							append(configurers,
								c.withFlagRequestMethod(method),
								transport.configurer,
							)...)...)

						request := dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)
						m.In(t).For("request method").Assert(request.Method, m.Equal(string(method)))
						m.In(t).For("request headers").Assert(request.Headers, c.authorizationHeaderMatcher(credential))
					})
				}
			})
		}
	})
	t.Run("invalid tls certificate", func(t *ldtest.T) {
		c.withHTTPSTransport(t).Run(t, func(t *ldtest.T) {
			dataSystem, configurers := c.setupDataSystems(t, nil)

			_ = NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)

			_, err := dataSystem.Synchronizers[0].Endpoint().AwaitConnection(time.Second)
			assert.Errorf(t, err, "expected connection error")
		})
	})
}

func (c CommonStreamingTests) RequestURLPath(t *ldtest.T, pathMatcher func(flagRequestMethod) m.Matcher) {
	t.Run("URL path is computed correctly", func(t *ldtest.T) {
		for _, trailingSlash := range []bool{false, true} {
			t.Run(h.IfElse(trailingSlash, "base URI has a trailing slash",
				"base URI has no trailing slash"), func(t *ldtest.T) {
				for _, method := range c.availableFlagRequestMethods(t) {
					t.Run(string(method), func(t *ldtest.T) {
						dataSystem, configurers := c.setupDataSystems(t, nil)

						streamURI := strings.TrimSuffix(dataSystem.Synchronizers[0].Endpoint().BaseURL(), "/")
						if trailingSlash {
							streamURI += "/"
						}

						var uriConfigurer SDKConfigurer
						if c.isClientSide {
							uriConfigurer = WithConnectionModeSynchronizer("streaming", servicedef.DataSynchronizer{
								Streaming: o.Some(servicedef.SDKConfigStreamingParams{BaseURI: streamURI}),
							})
						} else {
							uriConfigurer = WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
								BaseURI: streamURI,
							})
						}
						_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
							append(configurers,
								uriConfigurer,
								c.withFlagRequestMethod(method),
							)...)...)

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
							dataSystem, configurers := c.setupDataSystems(t, nil)

							_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
								append(configurers,
									WithClientSideConfig(servicedef.SDKConfigClientSideParams{
										EvaluationReasons: withReasons,
										InitialContext:    o.Some(ldcontext.New("irrelevant-key")),
									}),
									c.withFlagRequestMethod(method),
								)...)...)

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

func (c CommonStreamingTests) RequestContextProperties(t *ldtest.T, getPath string) {
	t.RequireCapability(servicedef.CapabilityClientSide) // server-side SDKs do not send user properties in stream requests

	t.Run("context properties", func(t *ldtest.T) {
		for _, contexts := range data.NewContextFactoriesForExercisingAllAttributes(c.contextFactory.Prefix()) {
			t.Run(contexts.Description(), func(t *ldtest.T) {
				for _, method := range c.availableFlagRequestMethods(t) {
					t.Run(string(method), func(t *ldtest.T) {
						dataSystem, configurers := c.setupDataSystems(t, nil)

						context := contexts.NextUniqueContext()
						contextJSONMatcher := JSONMatchesContext(context)

						_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
							append(configurers,
								WithClientSideInitialContext(context),
								c.withFlagRequestMethod(method),
							)...)...)

						request := dataSystem.Synchronizers[0].Endpoint().RequireConnection(t, time.Second)

						if method.sendsContextInBody() {
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

func (c CommonStreamingTests) RequestViaHTTPProxy(t *ldtest.T) {
	t.Run("http proxy", func(t *ldtest.T) {
		dataSystem, configurers := c.setupDataSystems(t, nil)

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
		streamURI := strings.Replace(dataSystem.Synchronizers[0].Endpoint().BaseURL(), "localhost", "not.valid.local", 1)

		u, err := url.Parse(dataSystem.Synchronizers[0].Endpoint().BaseURL())
		if err != nil {
			t.Errorf("unexpected error parsing URL: %s", err)
			t.FailNow()
		}
		u.Path = ""

		var uriConfigurer SDKConfigurer
		if c.isClientSide {
			uriConfigurer = WithConnectionModeSynchronizer("streaming", servicedef.DataSynchronizer{
				Streaming: o.Some(servicedef.SDKConfigStreamingParams{BaseURI: streamURI}),
			})
		} else {
			uriConfigurer = WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
				BaseURI: streamURI,
			})
		}

		_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
			append(configurers,
				uriConfigurer,
				c.withHTTPProxy(u.String()),
			)...)...)

		_, err = dataSystem.Synchronizers[0].Endpoint().AwaitConnection(time.Second)
		assert.NoError(t, err)
	})
}
