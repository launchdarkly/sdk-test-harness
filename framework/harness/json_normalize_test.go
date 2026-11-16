package harness

import (
	"encoding/json"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeSurrogateEscapes(t *testing.T) {
	escaped := []byte(`{"value": "caf\u00e9 \ud83d\ude00", "n": 1.50, "html": "<&>"}`)
	normalized := normalizeSurrogateEscapes(escaped)

	var resp struct {
		Value ldvalue.Value `json:"value"`
		N     json.Number   `json:"n"`
		HTML  string        `json:"html"`
	}
	require.NoError(t, json.Unmarshal(normalized, &resp))
	assert.Equal(t, "caf\u00e9 \U0001F600", resp.Value.StringValue())
	assert.Equal(t, json.Number("1.50"), resp.N)
	assert.Equal(t, "<&>", resp.HTML)
}

func TestNormalizeSurrogateEscapesLeavesOtherInputUnchanged(t *testing.T) {
	for _, s := range []string{`{"value": "caf\u00e9"}`, `not JSON \ud83d`, ``} {
		assert.Equal(t, s, string(normalizeSurrogateEscapes([]byte(s))))
	}
}
