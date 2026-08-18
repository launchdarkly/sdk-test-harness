package sdktests

import (
	"bytes"
	"net/http"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"

	"github.com/stretchr/testify/require"
)

// doServerSidePollUndecodablePayloadTests checks that an SDK does not silently corrupt flag data
// when a polling response body cannot be decoded as valid UTF-8.
//
// The mock polling service serves a body that is valid JSON except for a single invalid UTF-8 byte
// inside a flag's string value. An SDK that decodes strictly must reject the whole payload and keep
// no flag data. An SDK that decodes with byte replacement turns the invalid byte into U+FFFD, parses
// the now-valid JSON, and applies a corrupted flag value -- the bug this test detects.
//
// The observable signal is the evaluated flag value. The flag is off with an off-variation whose
// value is the corrupted string, so:
//   - a strict SDK never initializes from this payload and returns the evaluation default;
//   - a lenient SDK initializes and returns the U+FFFD-corrupted string.
//
// The test asserts the SDK returns the default, i.e. it did not apply data from an undecodable payload.
// It runs for every polling-capable server-side SDK; the enclosing polling group already requires the
// server-side-polling capability.
func doServerSidePollUndecodablePayloadTests(t *ldtest.T) {
	const flagKey = "flag"
	const sentinel = "SENTINELVALUE"
	defaultValue := ldvalue.String("default-fallback")
	context := ldcontext.New("user-key")

	// Build a valid server-side polling payload with a single flag whose off-variation value is the
	// sentinel string, then splice one invalid UTF-8 byte (0xFF) into the middle of that string. The
	// result is still structurally valid JSON: 0xFF sits inside a JSON string, so a lenient decoder
	// that replaces it with U+FFFD produces parseable JSON, while a strict decoder raises.
	flag := ldbuilders.NewFlagBuilder(flagKey).Version(1).
		On(false).OffVariation(0).Variations(ldvalue.String(sentinel)).Build()
	validBody := mockld.NewServerSDKDataBuilder().Flag(flag).Build().Serialize()
	badBody := bytes.Replace(validBody, []byte(sentinel), []byte("AB\xffCD"), 1)
	require.NotEqual(t, validBody, badBody, "test setup failed to inject the invalid byte")

	// The value a lenient (errors='replace') SDK would apply after substituting U+FFFD for 0xFF.
	corruptedValue := ldvalue.String("AB�CD")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(badBody)
	})
	endpoint := requireContext(t).harness.NewMockEndpoint(handler, t.DebugLogger(),
		harness.MockEndpointDescription("polling service with undecodable payload"))
	t.Defer(endpoint.Close)

	// InitCanFail lets the client be created even when the data source never initializes, so that a
	// correct (strict) SDK can still answer evaluations with the default. A short start-wait keeps the
	// strict SDK from blocking for a long time while its first poll fails.
	sdkKey := "my-sdk-key"
	client := NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{
		Credential:      sdkKey,
		InitCanFail:     true,
		StartWaitTimeMS: o.Some(ldtime.UnixMillisecondTime(2000)),
		Polling: o.Some(servicedef.SDKConfigPollingParams{
			BaseURI: endpoint.BaseURL(),
		}),
	}))

	result := evaluateFlagDetail(t, client, flagKey, context, defaultValue)

	if result.Value.Equal(corruptedValue) {
		require.Fail(t, "SDK applied data from an undecodable payload",
			"the SDK returned the U+FFFD-corrupted value %s from a body that failed to decode;"+
				" it should have rejected the payload and returned the default %s (reason: %+v)",
			result.Value, defaultValue, result.Reason.Value())
	}
	require.True(t, result.Value.Equal(defaultValue),
		"expected the default value %s (payload rejected), but got %s (reason: %+v)",
		defaultValue, result.Value, result.Reason.Value())
}
