package sdktests

import (
	h "github.com/launchdarkly/sdk-test-harness/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
)

func doClientSideEventTests(t *ldtest.T) {
	t.Run("requests", doClientSideEventRequestTests)
	t.Run("identify events", doClientSideEventIdentifyTests)
	t.Run("event capacity", doClientSideEventBufferTests)
	t.Run("disabling", doClientSideEventDisableTests)
}

func doClientSideEventRequestTests(t *ldtest.T) {
	sdkKind := requireContext(t).sdkKind
	users := NewUserFactory("doClientSideEventRequestTests")
	envIDOrMobileKey := "my-credential"

	commonTests := CommonEventTests{
		SDKConfigurers: []SDKConfigurer{
			WithConfig(servicedef.SDKConfigParams{
				Credential: envIDOrMobileKey,
			}),
			WithClientSideConfig(servicedef.SDKConfigClientSideParams{
				InitialUser: users.NextUniqueUser(),
			}),
		},
	}

	commonTests.RequestMethodAndHeaders(t,
		Header("Authorization").Should(m.Equal(
			h.IfElse(sdkKind == mockld.MobileSDK, envIDOrMobileKey, ""),
		)))

	requestPathMatcher := h.IfElse(
		sdkKind == mockld.JSClientSDK,
		m.Equal("/events/bulk/"+envIDOrMobileKey),
		m.AnyOf(
			// for mobile, there are several supported paths
			m.Equal("/mobile"),
			m.Equal("/mobile/events"),
			m.Equal("/mobile/events/bulk"),
		),
	)
	commonTests.RequestURLPath(t, requestPathMatcher)

	commonTests.UniquePayloadIDs(t)
}

func doClientSideEventIdentifyTests(t *ldtest.T) {
	users := NewUserFactory("doClientSideEventIdentifyTests")

	commonTests := CommonEventTests{
		SDKConfigurers: []SDKConfigurer{
			WithConfig(servicedef.SDKConfigParams{
				Credential: "my-credential",
			}),
			WithClientSideConfig(servicedef.SDKConfigClientSideParams{
				InitialUser: users.NextUniqueUser(),
			}),
		},
	}

	commonTests.IdentifyEvents(t, users)
}

func doClientSideEventBufferTests(t *ldtest.T) {
	users := NewUserFactory("doClientSideEventBufferTests")

	commonTests := CommonEventTests{
		SDKConfigurers: []SDKConfigurer{
			WithClientSideConfig(servicedef.SDKConfigClientSideParams{
				InitialUser: users.NextUniqueUser(),
			}),
		},
	}

	commonTests.BufferBehavior(t, users)
}

func doClientSideEventDisableTests(t *ldtest.T) {
	commonTests := CommonEventTests{
		SDKConfigurers: []SDKConfigurer{
			WithClientSideConfig(servicedef.SDKConfigClientSideParams{
				InitialUser: lduser.NewUser("initial-user"),
			}),
		},
	}

	commonTests.DisablingEvents(t)
}
