package sdktests

import (
	"strings"
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"github.com/stretchr/testify/assert"
)

func doServerSideStreamRequestTests(t *ldtest.T) {
	t.Run("headers", func(t *ldtest.T) {
		sdkKey := "my-sdk-key"

		dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
		_ = NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{
			Credential: sdkKey,
		}), dataSource)

		request := expectRequest(t, dataSource.Endpoint(), time.Second)

		assert.Equal(t, sdkKey, request.Headers.Get("Authorization"))
		assert.NotEqual(t, sdkKey, request.Headers.Get("User-Agent"))
	})

	t.Run("URL path is correct when base URI has a trailing slash", func(t *ldtest.T) {
		dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
		_ = NewSDKClient(t, WithStreamingConfig(servicedef.SDKConfigStreamingParams{
			BaseURI: strings.TrimSuffix(dataSource.Endpoint().BaseURL(), "/") + "/",
		}))

		request := expectRequest(t, dataSource.Endpoint(), time.Second)
		assert.Equal(t, "/all", request.URL.Path)
	})

	t.Run("URL path is correct when base URI has no trailing slash", func(t *ldtest.T) {
		dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
		_ = NewSDKClient(t, WithStreamingConfig(servicedef.SDKConfigStreamingParams{
			BaseURI: strings.TrimSuffix(dataSource.Endpoint().BaseURL(), "/"),
		}))

		request := expectRequest(t, dataSource.Endpoint(), time.Second)
		assert.Equal(t, "/all", request.URL.Path)
	})
}
