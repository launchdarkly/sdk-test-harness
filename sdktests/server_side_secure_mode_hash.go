package sdktests

import (
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"

	"github.com/stretchr/testify/assert"
)

func doServerSideSecureModeHashTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilitySecureModeHash)

	// These parameters were obtained by manual testing of a GA release of go-server-sdk that is
	// being used as a reference implementation.
	sdkKey1, sdkKey2 := "sdk-01234567-89ab-cdef-0123-456789abcdef", "sdk-11234567-89ab-cdef-0123-456789abcdef"
	userKey1, userKey2 := "user-key-123", "user-key-456"
	allParams := []struct {
		sdkKey       string
		user         lduser.User
		expectedHash string
	}{
		{
			sdkKey:       sdkKey1,
			user:         lduser.NewUser(userKey1),
			expectedHash: "73df666a13f2c474e50aa34ca5a761e89abb737fb139ff65fdde7fa85c9dcacd",
		},
		{
			// same SDK key with same user key = same result, regardless of other user attributes
			sdkKey: sdkKey1,
			user: lduser.NewUserBuilder(userKey1).
				Anonymous(true).Avatar("a").Country("b").Email("c").FirstName("d").IP("e").
				LastName("f").Name("g").Secondary("h").Custom("i", ldvalue.String("j")).Build(),
			expectedHash: "73df666a13f2c474e50aa34ca5a761e89abb737fb139ff65fdde7fa85c9dcacd",
		},
		{
			// different SDK key with same user key = different result
			sdkKey:       sdkKey2,
			user:         lduser.NewUser(userKey1),
			expectedHash: "63538426a9845721a5547b4715f4284b060c21743702a896e1ff8a9a5b57215d",
		},
		{
			// same SDK key with different user key = different result
			sdkKey:       sdkKey1,
			user:         lduser.NewUser(userKey2),
			expectedHash: "55be6b5ceb2a11acc6a7e9c60dbee5022d9c7084baf9ecd8cf69d12bce5a92fb",
		},
	}

	dataSource := NewSDKDataSource(t, nil)
	for i, p := range allParams {
		t.Run(fmt.Sprintf("test case %d", i+1), func(t *ldtest.T) {
			client := NewSDKClient(t, WithCredential(p.sdkKey), dataSource)
			hash := client.GetSecureModeHash(t, p.user)
			assert.Equal(t, p.expectedHash, hash)
		})
	}
}
