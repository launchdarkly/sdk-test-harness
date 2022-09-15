package sdktests

import (
	"fmt"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v2/ldbuilders"
	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
	"net/http"
	"strings"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/data"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

func (c CommonPollingTests) RequestMethodAndHeaders(t *ldtest.T, credential string) {
	t.Run("method and headers", func(t *ldtest.T) {
		for _, method := range c.availableFlagRequestMethods() {
			t.Run(string(method), func(t *ldtest.T) {
				dataSource := NewSDKDataSource(t, nil, DataSourceOptionPolling())
				_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
					c.withFlagRequestMethod(method),
					dataSource)...)

				request := dataSource.Endpoint().RequireConnection(t, time.Second)
				m.In(t).For("request method").Assert(request.Method, m.Equal(string(method)))
				m.In(t).For("request headers").Assert(request.Headers, c.authorizationHeaderMatcher(credential))
			})
		}
	})
}

func (c CommonPollingTests) RequestURLPath(t *ldtest.T, pathMatcher func(flagRequestMethod) m.Matcher) {
	t.Run("URL path is computed correctly", func(t *ldtest.T) {
		for _, trailingSlash := range []bool{false, true} {
			t.Run(h.IfElse(trailingSlash, "base URI has a trailing slash", "base URI has no trailing slash"), func(t *ldtest.T) {
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
							}),
						)...)

						request := dataSource.Endpoint().RequireConnection(t, time.Second)
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
					for _, method := range c.availableFlagRequestMethods() {
						t.Run(string(method), func(t *ldtest.T) {
							dataSource := NewSDKDataSource(t, nil, DataSourceOptionPolling())

							_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
								c.withFlagRequestMethod(method),
								WithClientSideConfig(servicedef.SDKConfigClientSideParams{
									EvaluationReasons: withReasons,
									InitialContext:    ldcontext.New("irrelevant-key"),
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

func (c CommonPollingTests) ShouldRetryAfterError(t *ldtest.T) {
	shouldRetryAfterErrorOnInitialConnection := func(t *ldtest.T, errorHandler http.Handler) {
		context := ldcontext.New("user-key")
		expectedValueV1 := ldvalue.Int(1)
		flagKey := "flag"
		flag := ldbuilders.NewFlagBuilder(flagKey).Version(1).
			On(false).OffVariation(0).Variations(expectedValueV1).Build()
		data := mockld.NewServerSDKDataBuilder().Flag(flag).Build()
		dataSource := NewSDKDataSource(t, data, DataSourceOptionPolling())
		handler := httphelpers.SequentialHandler(
			errorHandler,
			dataSource.pollingService,
		)
		pollingEndpoint := makePollEndpoint(t, handler)
		t.Defer(pollingEndpoint.Close)

		client := NewSDKClient(t, c.baseSDKConfigurationPlus(
			WithConfig(servicedef.SDKConfigParams{InitCanFail: true}),
			c.withFlagRequestMethod(c.availableFlagRequestMethods()[0]),
			WithPollingConfig(servicedef.SDKConfigPollingParams{
				BaseURI:        pollingEndpoint.BaseURL(),
				PollIntervalMS: o.Some(ldtime.UnixMillisecondTime(0)),
			}),
		)...)

		incomingConnectionTimeout := time.Second * 2
		// Error
		pollingEndpoint.RequireConnection(t, incomingConnectionTimeout)
		// Success
		pollingEndpoint.RequireConnection(t, time.Second*60)

		result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{Context: o.Some(context)})
		m.In(t).Assert(result, EvalAllFlagsValueForKeyShouldEqual(flagKey, expectedValueV1))

		// Shouldn't get any more connections.
		pollingEndpoint.RequireNoMoreConnections(t, incomingConnectionTimeout)
	}

	t.Run("retry after IO error on initial connect", func(t *ldtest.T) {
		shouldRetryAfterErrorOnInitialConnection(t, httphelpers.BrokenConnectionHandler())
	})
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
							WithClientSideConfig(servicedef.SDKConfigClientSideParams{
								InitialContext: context,
							}),
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
