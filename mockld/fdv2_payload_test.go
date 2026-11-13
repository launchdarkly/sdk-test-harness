package mockld

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/launchdarkly/sdk-test-harness/v3/framework"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"

	"github.com/stretchr/testify/require"
)

func TestFDv2PayloadEnvelope(t *testing.T) {
	for _, state := range []string{"", "initial"} {
		for _, withObjects := range []bool{false, true} {
			t.Run(fmt.Sprintf("state=%q/objects=%t", state, withObjects), func(t *testing.T) {
				var objects []framework.BaseObject
				if withObjects {
					objects = []framework.BaseObject{{
						Kind: "flag", Key: "flag-key", Version: 5,
						Object: json.RawMessage(`{"key":"flag-key"}`),
					}}
				}
				id, version := "payloadID", 1
				if state == "" {
					id, version = "", 0
				}
				intent := fmt.Sprintf(
					`{"payloads":[{"id":%q,"target":%d,"intentCode":"xfer-full","reason":"cant-catchup"}]}`,
					id, version)
				transferred := fmt.Sprintf(`{"state":%q,"version":%d}`, state, version)
				data := NewFDv2SDKData("xfer-full", "cant-catchup", state, objects)
				testLog := ldlogtest.NewMockLog()
				defer testLog.DumpIfTestFailed(t)
				logger := testLog.Loggers.ForLevel(ldlog.Debug)

				t.Run("streaming", func(t *testing.T) {
					service := NewStreamingService(data, ServerSideSDK, logger)
					events := service.makeXferFull()
					require.Len(t, events, len(objects)+2)
					require.Equal(t, "server-intent", events[0].Event())
					require.JSONEq(t, intent, events[0].Data())
					require.Equal(t, "payload-transferred", events[len(events)-1].Event())
					require.JSONEq(t, transferred, events[len(events)-1].Data())
					if withObjects {
						require.Equal(t, "put-object", events[1].Event())
						require.JSONEq(t,
							`{"kind":"flag","key":"flag-key","version":5,"object":{"key":"flag-key"}}`,
							events[1].Data())
					}
				})

				t.Run("polling", func(t *testing.T) {
					service := NewPollingService(data, ServerSideSDK, logger)
					response := httptest.NewRecorder()
					service.ServeHTTP(response, httptest.NewRequest(http.MethodGet, PollingPathServerSide, nil))
					require.Equal(t, http.StatusOK, response.Code)
					var payload struct {
						Events []struct {
							Event string          `json:"event"`
							Data  json.RawMessage `json:"data"`
						} `json:"events"`
					}
					require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
					require.Len(t, payload.Events, len(objects)+2)
					require.Equal(t, "server-intent", payload.Events[0].Event)
					require.JSONEq(t, intent, string(payload.Events[0].Data))
					require.Equal(t, "payload-transferred", payload.Events[len(payload.Events)-1].Event)
					require.JSONEq(t, transferred, string(payload.Events[len(payload.Events)-1].Data))
					if withObjects {
						require.Equal(t, "put-object", payload.Events[1].Event)
						require.JSONEq(t,
							`{"kind":"flag","key":"flag-key","version":5,"object":{"key":"flag-key"}}`,
							string(payload.Events[1].Data))
					}
				})
			})
		}
	}
}
