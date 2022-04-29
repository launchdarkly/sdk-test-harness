package sdktests

import (
	"strings"
	"time"

	h "github.com/launchdarkly/sdk-test-harness/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

func (c CommonStreamingTests) RequestMethodAndHeaders(t *ldtest.T, headersMatcher m.Matcher) {
	t.Run("method and headers", func(t *ldtest.T) {
		for _, method := range c.availableFlagRequestMethods() {
			t.Run(string(method), func(t *ldtest.T) {
				dataSource, configurers := c.setupDataSources(t, nil)

				_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
					append(configurers,
						c.withFlagRequestMethod(method),
					)...)...)

				request := dataSource.Endpoint().RequireConnection(t, time.Second)
				m.In(t).For("request method").Assert(request.Method, m.Equal(string(method)))
				m.In(t).For("request headers").Assert(request.Headers, headersMatcher)
			})
		}
	})
}

func (c CommonStreamingTests) RequestURLPath(t *ldtest.T, pathMatcher func(flagRequestMethod) m.Matcher) {
	t.Run("URL path is computed correctly", func(t *ldtest.T) {
		for _, trailingSlash := range []bool{false, true} {
			t.Run(h.IfElse(trailingSlash, "base URI has a trailing slash", "base URI has no trailing slash"), func(t *ldtest.T) {
				for _, method := range c.availableFlagRequestMethods() {
					t.Run(string(method), func(t *ldtest.T) {
						dataSource, configurers := c.setupDataSources(t, nil)

						streamURI := strings.TrimSuffix(dataSource.Endpoint().BaseURL(), "/")
						if trailingSlash {
							streamURI += "/"
						}

						_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
							append(configurers,
								WithStreamingConfig(servicedef.SDKConfigStreamingParams{
									BaseURI: streamURI,
								}),
								c.withFlagRequestMethod(method),
							)...)...)

						request := dataSource.Endpoint().RequireConnection(t, time.Second)
						m.In(t).For("request path").Assert(request.URL.Path, pathMatcher(method))
					})
				}
			})
		}
	})
}

func (c CommonStreamingTests) RequestUserProperties(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityClientSide) // server-side SDKs do not send user properties in stream requests

	t.Run("user properties", func(t *ldtest.T) {
		for _, method := range c.availableFlagRequestMethods() {
			t.Run(string(method), func(t *ldtest.T) {
				dataSource, configurers := c.setupDataSources(t, nil)

				user := lduser.NewUserBuilder(c.userFactory.NextUniqueUser().GetKey()).
					Name("a").
					Email("b").AsPrivateAttribute().
					Custom("c", ldvalue.String("d")).
					Build()
				userJSONMatcher := m.JSONMap().Should(m.MapIncluding(
					m.KV("key", m.Equal(user.GetKey())),
					m.KV("name", m.Equal("a")),
					m.KV("email", m.Equal("b")),
					m.KV("custom", m.MapOf(
						m.KV("c", m.Equal("d")),
					)),
					m.KV("privateAttributeNames", m.Items(m.Equal("email"))),
				))
				_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
					append(configurers,
						WithClientSideConfig(servicedef.SDKConfigClientSideParams{
							InitialUser: user,
						}),
						c.withFlagRequestMethod(method),
					)...)...)

				request := dataSource.Endpoint().RequireConnection(t, time.Second)

				if method == flagRequestREPORT {
					m.In(t).For("request body").Assert(request.Body, m.AllOf(
						m.Not(m.BeNil()),
						userJSONMatcher))
				} else {
					m.In(t).For("request body").Assert(request.Body, m.Length().Should(m.Equal(0)))

					pathParts := strings.Split(strings.TrimPrefix(request.URL.Path, "/"), "/")
					expectedPathComponents := h.IfElse(c.sdkKind == mockld.JSClientSDK, 3, 2)
					if len(pathParts) != expectedPathComponents {
						t.Errorf("expected URL path with %d components but got: %s", expectedPathComponents, request.URL.Path)
						t.FailNow()
					}

					m.In(t).For("user data in URL").Assert(pathParts[len(pathParts)-1],
						Base64DecodedData().Should(userJSONMatcher))
				}
			})
		}
	})
}
