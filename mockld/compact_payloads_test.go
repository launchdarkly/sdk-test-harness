package mockld

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/launchdarkly/sdk-test-harness/v3/framework"

	"github.com/launchdarkly/go-jsonstream/v3/jwriter"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeCompactTestFlag() ldmodel.FeatureFlag {
	return ldbuilders.NewFlagBuilder("flag1").Version(2).On(true).
		Variations(ldvalue.Bool(false), ldvalue.Bool(true)).
		OffVariation(0).FallthroughVariation(1).
		AddRule(ldbuilders.NewRuleBuilder().ID("rule1").Variation(0).
			Clauses(ldbuilders.Clause("name", ldmodel.OperatorIn, ldvalue.String("x")))).
		Salt("salt").
		Build()
}

func makeCompactTestSegment() ldmodel.Segment {
	return ldbuilders.NewSegmentBuilder("segment1").Version(3).Included("a").Salt("salt").Build()
}

func TestCompactObjectJSONForFlag(t *testing.T) {
	flag := makeCompactTestFlag()
	writer := jwriter.NewWriter()
	ldmodel.MarshalFeatureFlagToJSONWriterCompact(flag, &writer)
	require.NoError(t, writer.Error())

	compact := CompactObjectJSON("flag", jsonhelpers.ToJSON(flag))
	assert.JSONEq(t, string(writer.Bytes()), string(compact))
	assert.Less(t, len(compact), len(jsonhelpers.ToJSON(flag)))
}

func TestCompactObjectJSONForSegment(t *testing.T) {
	segment := makeCompactTestSegment()
	writer := jwriter.NewWriter()
	ldmodel.MarshalSegmentToJSONWriterCompact(segment, &writer)
	require.NoError(t, writer.Error())

	compact := CompactObjectJSON("segment", jsonhelpers.ToJSON(segment))
	assert.JSONEq(t, string(writer.Bytes()), string(compact))
}

func TestCompactObjectJSONDoesNotChangeObjectsThatAreNotFullEncodings(t *testing.T) {
	for _, p := range []struct {
		name string
		kind string
		data string
	}{
		{"flag with a property of the wrong type", "flag", `{"key": "flag1", "on": "yes"}`},
		{"flag that is not in the full encoding", "flag", `{"key": "flag1", "version": 1}`},
		{"segment that is not in the full encoding", "segment", `{"key": "segment1", "version": 1}`},
		{"flag that is not an object", "flag", `[1, 2]`},
		{"flag evaluation that is not an object", "flag-eval", `"x"`},
		{"unknown kind", "other", `{"value": null}`},
	} {
		t.Run(p.name, func(t *testing.T) {
			assert.Equal(t, p.data, string(CompactObjectJSON(p.kind, json.RawMessage(p.data))))
		})
	}
}

func TestCompactObjectJSONForFlagEvaluation(t *testing.T) {
	for _, p := range []struct {
		name     string
		data     string
		expected string
	}{
		{
			"removes null values and false booleans, and keeps value, variation, and flagVersion",
			`{"value": null, "variation": null, "reason": null, "flagVersion": null, "samplingRatio": null,
				"debugEventsUntilDate": null, "prerequisites": null, "trackEvents": false, "trackReason": false,
				"version": 1}`,
			`{"value": null, "variation": null, "flagVersion": null, "version": 1}`,
		},
		{
			"keeps a false value, a zero variation, and true booleans",
			`{"value": false, "variation": 0, "trackEvents": true, "trackReason": true, "version": 1}`,
			`{"value": false, "variation": 0, "trackEvents": true, "trackReason": true, "version": 1}`,
		},
		{
			"removes empty prerequisites and keeps others",
			`{"value": 1, "prerequisites": [], "flagVersion": 0, "samplingRatio": 0, "version": 1}`,
			`{"value": 1, "flagVersion": 0, "samplingRatio": 0, "version": 1}`,
		},
		{
			"keeps a non-empty prerequisites list",
			`{"value": 1, "prerequisites": ["a"], "version": 1}`,
			`{"value": 1, "prerequisites": ["a"], "version": 1}`,
		},
	} {
		t.Run(p.name, func(t *testing.T) {
			assert.JSONEq(t, p.expected, string(CompactObjectJSON("flag-eval", json.RawMessage(p.data))))
		})
	}
}

func TestCompactPutObjectIf(t *testing.T) {
	fullJSON := jsonhelpers.ToJSON(makeCompactTestFlag())
	obj := framework.BaseObject{Version: 1, Kind: "flag", Key: "flag1", Object: fullJSON}

	t.Run("not compact", func(t *testing.T) {
		assert.Equal(t, obj, compactPutObjectIf(false, obj))
	})

	t.Run("compact", func(t *testing.T) {
		result := compactPutObjectIf(true, obj)
		assert.JSONEq(t, string(CompactObjectJSON("flag", fullJSON)), string(result.Object))
		assert.Equal(t, obj.Key, result.Key)
		assert.Equal(t, obj.Version, result.Version)
	})

	t.Run("compact, object without content", func(t *testing.T) {
		deleted := framework.BaseObject{Version: 2, Kind: "flag", Key: "flag1"}
		assert.Equal(t, deleted, compactPutObjectIf(true, deleted))
	})
}

func TestAsksForCompactPayloads(t *testing.T) {
	for query, expected := range map[string]bool{
		"":                        false,
		"?enc=compact":            true,
		"?basis=x&enc=compact":    true,
		"?enc=full":               false,
		"?filter=f&withReasons=1": false,
	} {
		r := httptest.NewRequest(http.MethodGet, "/sdk/stream"+query, nil)
		assert.Equal(t, expected, asksForCompactPayloads(r), query)
	}
}

func TestStreamingPutObjectEventFollowsRequest(t *testing.T) {
	fullJSON := jsonhelpers.ToJSON(makeCompactTestFlag())
	obj := framework.BaseObject{Version: 1, Kind: "flag", Key: "flag1", Object: fullJSON}
	compact := &atomic.Bool{}
	event := eventImpl{event: "put-object", data: obj, compact: compact}

	assert.JSONEq(t, string(jsonhelpers.ToJSON(obj)), event.Data())

	compact.Store(true)
	assert.JSONEq(t, string(jsonhelpers.ToJSON(compactPutObject(obj))), event.Data())
}

func TestPollingPutObjectEventFollowsRequest(t *testing.T) {
	fullJSON := jsonhelpers.ToJSON(makeCompactTestFlag())
	data := FDv2SDKDataFromServerSDKData(
		NewServerSDKDataBuilder().Flag(makeCompactTestFlag()).Build(), "xfer-full", "initial", "initial")
	service := NewPollingService(data, ServerSideSDK, ldlogtest.NewMockLog().Loggers.ForLevel(ldlog.Debug))

	for query, expected := range map[string]json.RawMessage{
		"":             fullJSON,
		"?enc=compact": CompactObjectJSON("flag", fullJSON),
	} {
		response := httptest.NewRecorder()
		service.ServeHTTP(response, httptest.NewRequest(http.MethodGet, PollingPathServerSide+query, nil))
		require.Equal(t, http.StatusOK, response.Code)

		var payload struct {
			Events []struct {
				Event string               `json:"event"`
				Data  framework.BaseObject `json:"data"`
			} `json:"events"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
		require.Len(t, payload.Events, 3)
		assert.Equal(t, "put-object", payload.Events[1].Event)
		assert.JSONEq(t, string(expected), string(payload.Events[1].Data.Object), query)
	}
}
