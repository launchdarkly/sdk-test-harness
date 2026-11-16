package sdktests

import (
	"encoding/json"
	"fmt"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"
)

// doStoredEncodingTests checks the encoding of the flags that the SDK writes to the store. With
// compact payloads on, the SDK writes the compact encoding. With compact payloads off, the SDK
// writes the full encoding.
//
// The test flag is off and has no rules. The compact encoding omits both properties. The full
// encoding includes both.
func (s *ServerSidePersistentTests) doStoredEncodingTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityCompactPayloads)

	compact := compactPayloadsMode.Load()
	name := "writes the full encoding when compact payloads are off"
	shape := m.AllOf(jsonHasProperty("on"), jsonHasProperty("rules"))
	if compact {
		name = "writes the compact encoding when compact payloads are on"
		shape = m.AllOf(m.Not(jsonHasProperty("on")), m.Not(jsonHasProperty("rules")))
	}

	s.runWithEmptyStore(t, name, func(t *ldtest.T) {
		sdkData := s.makeSDKDataWithFlag(1, ldvalue.String("value"))
		_, configurers := s.setupDataSystems(t, sdkData)

		_ = s.newDataSourceClient(t, configurers...)

		s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)
		s.eventuallyValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
			"flag-key": m.AllOf(basicFlagValidationMatcher("flag-key", 1, "value"), shape),
		})
	})
}

// jsonHasProperty matches a JSON object, in a string, that has the named property.
func jsonHasProperty(name string) m.Matcher {
	return m.New(
		func(value any) bool {
			text, ok := value.(string)
			if !ok {
				return false
			}
			var obj map[string]json.RawMessage
			if json.Unmarshal([]byte(text), &obj) != nil {
				return false
			}
			_, ok = obj[name]
			return ok
		},
		func() string { return fmt.Sprintf("has JSON property %q", name) },
		nil,
	)
}
