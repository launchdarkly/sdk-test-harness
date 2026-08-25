package servicedef

import (
	"encoding/json"
	"testing"

	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSDKConfigClientSideParamsSerializesBootstrap(t *testing.T) {
	params := SDKConfigClientSideParams{
		Bootstrap: o.Some(map[string]ldvalue.Value{
			"flagA":       ldvalue.Bool(true),
			"$flagsState": ldvalue.Parse([]byte(`{"flagA": {"variation": 1, "version": 100}}`)),
			"$valid":      ldvalue.Bool(true),
		}),
	}

	data, err := json.Marshal(params)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &fields))
	require.Contains(t, fields, "bootstrap")
	assert.JSONEq(t, `{
		"flagA": true,
		"$flagsState": {"flagA": {"variation": 1, "version": 100}},
		"$valid": true
	}`, string(fields["bootstrap"]))
}

func TestSDKConfigClientSideParamsSerializesBootstrapAsNullWhenUnset(t *testing.T) {
	data, err := json.Marshal(SDKConfigClientSideParams{})
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &fields))
	require.Contains(t, fields, "bootstrap")
	assert.JSONEq(t, `null`, string(fields["bootstrap"]))
}
