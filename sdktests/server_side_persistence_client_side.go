package sdktests

// A flag says which client-side SDKs can use it in two ways. The older clientSide property
// covers SDKs that use an environment ID. The newer clientSideAvailability object covers both
// SDKs that use an environment ID and SDKs that use a mobile key. When the object is absent, a
// reader uses the older rule: mobile key SDKs can use the flag, and environment ID SDKs can use
// it if clientSide is true. LaunchDarkly always sends the object.
//
// Readers such as the Relay Proxy serve client-side SDKs from a store that a server-side SDK
// wrote. These tests prove that the flag an SDK writes to the store has the same client-side
// availability as the flag that the SDK received, whether or not the object was present.

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	h "github.com/launchdarkly/sdk-test-harness/v3/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"
)

const clientSideFlagKey = "client-side-flag-key"

// clientSideAvailability is the availability that a flag gives to each kind of client-side SDK.
type clientSideAvailability struct {
	usingMobileKey     bool
	usingEnvironmentID bool
}

func (a clientSideAvailability) String() string {
	return fmt.Sprintf("usingMobileKey=%t, usingEnvironmentId=%t", a.usingMobileKey, a.usingEnvironmentID)
}

type clientSideAvailabilityCase struct {
	name string
	// fields are the client-side properties of the flag that the data source sends.
	fields map[string]interface{}
	want   clientSideAvailability
}

func clientSideAvailabilityCases() []clientSideAvailabilityCase {
	object := func(usingMobileKey, usingEnvironmentID bool) map[string]interface{} {
		return map[string]interface{}{
			"usingMobileKey":     usingMobileKey,
			"usingEnvironmentId": usingEnvironmentID,
		}
	}
	return []clientSideAvailabilityCase{
		{
			name:   "object with mobile key false and environment ID true",
			fields: map[string]interface{}{"clientSide": true, "clientSideAvailability": object(false, true)},
			want:   clientSideAvailability{usingMobileKey: false, usingEnvironmentID: true},
		},
		{
			name:   "object with mobile key true and environment ID false",
			fields: map[string]interface{}{"clientSide": false, "clientSideAvailability": object(true, false)},
			want:   clientSideAvailability{usingMobileKey: true, usingEnvironmentID: false},
		},
		{
			name:   "object with both false",
			fields: map[string]interface{}{"clientSide": false, "clientSideAvailability": object(false, false)},
			want:   clientSideAvailability{usingMobileKey: false, usingEnvironmentID: false},
		},
		{
			name:   "no object and clientSide true",
			fields: map[string]interface{}{"clientSide": true},
			want:   clientSideAvailability{usingMobileKey: true, usingEnvironmentID: true},
		},
		{
			name:   "no object and clientSide false",
			fields: map[string]interface{}{"clientSide": false},
			want:   clientSideAvailability{usingMobileKey: true, usingEnvironmentID: false},
		},
	}
}

func (s *ServerSidePersistentTests) doClientSideAvailabilityTests(t *ldtest.T) {
	for _, c := range clientSideAvailabilityCases() {
		s.runWithEmptyStore(t, c.name, func(t *ldtest.T) {
			flag := s.makeServerSideFlag(clientSideFlagKey, 1, ldvalue.String("value"))
			flagJSON := withJSONProperties(t, flag, c.fields)
			sdkData := mockld.NewServerSDKDataBuilder().RawFlag(clientSideFlagKey, flagJSON).Build()

			_, configurers := s.setupDataSystems(t, sdkData)
			persistence := NewPersistence()
			persistence.SetStoreMode(servicedef.DataStoreModeReadWrite)
			persistence.SetStore(servicedef.SDKConfigPersistentStore{
				Type: s.persistentStore.Type(),
				DSN:  s.persistentStore.DSN(),
			})
			persistence.SetCache(servicedef.SDKConfigPersistentCache{Mode: servicedef.CacheModeOff})

			_ = NewSDKClient(t, s.baseSDKConfigurationPlus(append(configurers, persistence)...)...)
			s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)
			s.eventuallyRequireClientSideAvailability(t, c.want)
		})
	}
}

// eventuallyRequireClientSideAvailability waits until the stored flag has the wanted availability.
func (s *ServerSidePersistentTests) eventuallyRequireClientSideAvailability(
	t *ldtest.T, want clientSideAvailability,
) {
	var lastStored string
	var lastErr error
	ok := h.AssertEventually(t, func() bool {
		data, err := s.persistentStore.GetMap(s.defaultPrefix, "features")
		if err != nil {
			lastErr = err
			return false
		}
		stored, found := data[clientSideFlagKey]
		if !found {
			lastErr = fmt.Errorf("the store has no %q flag", clientSideFlagKey)
			return false
		}
		lastStored = stored
		got, err := storedClientSideAvailability(stored)
		if err != nil {
			lastErr = err
			return false
		}
		lastErr = nil
		if got != want {
			lastErr = fmt.Errorf("stored flag has %s, want %s", got, want)
			return false
		}
		return true
	}, time.Second, time.Millisecond*20, "stored flag did not have the client-side availability of the flag sent")
	if !ok {
		t.Errorf("%v; stored flag: %s", lastErr, lastStored)
		t.FailNow()
	}
}

// storedClientSideAvailability reads a stored flag the way a reader of the store does.
func storedClientSideAvailability(flagJSON string) (clientSideAvailability, error) {
	var flag struct {
		ClientSide             *bool `json:"clientSide"`
		ClientSideAvailability *struct {
			UsingMobileKey     *bool `json:"usingMobileKey"`
			UsingEnvironmentID *bool `json:"usingEnvironmentId"`
		} `json:"clientSideAvailability"`
	}
	if err := json.Unmarshal([]byte(flagJSON), &flag); err != nil {
		return clientSideAvailability{}, fmt.Errorf("stored flag is not valid JSON: %w", err)
	}
	if flag.ClientSideAvailability == nil {
		return clientSideAvailability{
			usingMobileKey:     true,
			usingEnvironmentID: flag.ClientSide != nil && *flag.ClientSide,
		}, nil
	}
	if flag.ClientSideAvailability.UsingMobileKey == nil || flag.ClientSideAvailability.UsingEnvironmentID == nil {
		return clientSideAvailability{}, fmt.Errorf("stored clientSideAvailability does not have both properties")
	}
	return clientSideAvailability{
		usingMobileKey:     *flag.ClientSideAvailability.UsingMobileKey,
		usingEnvironmentID: *flag.ClientSideAvailability.UsingEnvironmentID,
	}, nil
}

// withJSONProperties returns the JSON of the item, without its client-side properties, plus the
// given properties.
func withJSONProperties(t *ldtest.T, item interface{}, properties map[string]interface{}) json.RawMessage {
	data, err := json.Marshal(item)
	require.NoError(t, err)
	var object map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &object))
	delete(object, "clientSide")
	delete(object, "clientSideAvailability")
	for name, value := range properties {
		object[name] = value
	}
	data, err = json.Marshal(object)
	require.NoError(t, err)
	return data
}
