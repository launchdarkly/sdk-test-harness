package sdktests

import (
	"fmt"
	"strings"
	"time"

	h "github.com/launchdarkly/sdk-test-harness/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"

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

func (c CommonPollingTests) RequestUserProperties(t *ldtest.T, getPath string) {
	t.RequireCapability(servicedef.CapabilityClientSide) // server-side SDKs do not send user properties in stream requests

	t.Run("user properties", func(t *ldtest.T) {
		for _, method := range c.availableFlagRequestMethods() {
			t.Run(string(method), func(t *ldtest.T) {
				dataSource := NewSDKDataSource(t, nil, DataSourceOptionPolling())

				user := lduser.NewUserBuilder(c.userFactory.NextUniqueUser().GetKey()).
					Name("a").
					Email("b").AsPrivateAttribute().
					Custom("c", ldvalue.String("d")).
					Build()
				userJSONMatcher := JSONMatchesUser(user, t.Capabilities().Has(servicedef.CapabilityMobile))

				_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
					WithClientSideConfig(servicedef.SDKConfigClientSideParams{
						InitialUser: user,
					}),
					c.withFlagRequestMethod(method),
					dataSource,
				)...)

				request := dataSource.Endpoint().RequireConnection(t, time.Second)

				if method == flagRequestREPORT {
					m.In(t).For("request body").Assert(request.Body, m.AllOf(
						m.Not(m.BeNil()),
						userJSONMatcher))
				} else {
					m.In(t).For("request body").Assert(request.Body, m.Length().Should(m.Equal(0)))

					getPathPrefix := strings.TrimSuffix(getPath, mockld.StreamingPathUserBase64Param)
					m.In(t).For("request path").Require(request.URL.Path, m.StringHasPrefix(getPathPrefix))
					userData := strings.TrimPrefix(request.URL.Path, getPathPrefix)

					m.In(t).For("user data in URL").Assert(userData,
						Base64DecodedData().Should(userJSONMatcher))
				}
			})
		}
	})
}
