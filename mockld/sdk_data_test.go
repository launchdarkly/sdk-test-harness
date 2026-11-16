package mockld

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerSDKDataNormalizesFlagsAndSegments(t *testing.T) {
	var data ServerSDKData
	require.NoError(t, json.Unmarshal([]byte(`{
		"flags": {"flag1": {"on": true, "variations": [1.0], "fallthrough": {"variation": 0}}},
		"segments": {"segment1": {"included": ["a"]}}
	}`), &data))

	flag := string(data["flag"]["flag1"])
	assert.Contains(t, flag, `"key":"flag1"`)
	assert.Contains(t, flag, `"version":1`)
	assert.Contains(t, flag, `"variations":[1]`)
	assert.Contains(t, flag, `"rules":[]`)

	segment := string(data["segment"]["segment1"])
	assert.Contains(t, segment, `"key":"segment1"`)
	assert.Contains(t, segment, `"excluded":[]`)
}

func TestServerSDKDataKeepsRawFlagsAndSegmentsUnchanged(t *testing.T) {
	rawFlag := `{"key": "flag1", "version": 2, "on": true, "variations": [1.0, 1E3, -0], ` +
		`"fallthrough": {"variation": 0}, "rules": null}`
	rawSegment := `{"key": "segment1", "version": 3}`
	input, _ := json.Marshal(map[string]interface{}{
		"rawFlags":    map[string]string{"flag1": rawFlag},
		"rawSegments": map[string]string{"segment1": rawSegment},
	})

	var data ServerSDKData
	require.NoError(t, json.Unmarshal(input, &data))
	assert.Equal(t, rawFlag, string(data["flag"]["flag1"]))
	assert.Equal(t, rawSegment, string(data["segment"]["segment1"]))

	serialized := string(data.Serialize())
	assert.Contains(t, serialized, `[1.0,1E3,-0]`)

	// The FDv2 put-object for each item carries the raw JSON as its object.
	objects := map[string]string{}
	for _, obj := range ServerSDKDataToFDv2Events(data) {
		objects[obj.Kind+"/"+obj.Key] = string(obj.Object)
		if obj.Kind == "flag" {
			eventData, err := json.Marshal(obj)
			require.NoError(t, err)
			assert.Contains(t, string(eventData), `[1.0,1E3,-0]`)
		}
	}
	assert.Equal(t, map[string]string{"flag/flag1": rawFlag, "segment/segment1": rawSegment}, objects)
}

func TestServerSDKDataRejectsBadRawItems(t *testing.T) {
	for name, input := range map[string]string{
		"raw flag is not a string":  `{"rawFlags": {"flag1": {"key": "flag1"}}}`,
		"raw flag is invalid JSON":  `{"rawFlags": {"flag1": "{\"key\": "}}`,
		"raw flag has wrong key":    `{"rawFlags": {"flag1": "{\"key\": \"flag2\"}"}}`,
		"raw flag has no key":       `{"rawFlags": {"flag1": "{\"version\": 1}"}}`,
		"raw flag is not an object": `{"rawFlags": {"flag1": "[\"flag1\"]"}}`,
		"raw segment has wrong key": `{"rawSegments": {"segment1": "{\"key\": \"segment2\"}"}}`,
		"flag defined twice": `{"flags": {"flag1": {}}, ` +
			`"rawFlags": {"flag1": "{\"key\": \"flag1\"}"}}`,
		"segment defined twice": `{"segments": {"segment1": {}}, ` +
			`"rawSegments": {"segment1": "{\"key\": \"segment1\"}"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			var data ServerSDKData
			assert.Error(t, json.Unmarshal([]byte(input), &data))
		})
	}
}
