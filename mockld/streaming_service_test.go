package mockld

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"

	"github.com/launchdarkly/eventsource"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sdkKey = "sdk-key"

func TestStreamingServiceServerSide(t *testing.T) {
	testLog := ldlogtest.NewMockLog()
	testLog.Loggers.SetMinLevel(ldlog.Debug)
	defer testLog.DumpIfTestFailed(t)

	initialData := EmptyServerSDKData()

	service := NewStreamingService(initialData, testLog.Loggers.ForLevel(ldlog.Debug))

	httphelpers.WithServer(service, func(server *httptest.Server) {
		endpointURL := server.URL + "/all"

		req, _ := http.NewRequest("GET", endpointURL, nil)
		req.Header.Set("Authorization", sdkKey)
		stream, err := eventsource.SubscribeWithRequest("", req)
		require.NoError(t, err)
		defer stream.Close()

		initialEvent := requireEvent(t, stream)
		assert.Equal(t, "put", initialEvent.Event())
		m.In(t).Assert(initialEvent.Data(), m.JSONStrEqual(expectedPutData(initialData)))

		newData := NewServerSDKDataBuilder().RawFlag("flag1", json.RawMessage(`{"key": "flag1"}`)).Build()
		go func() {
			service.SetInitialData(newData)
			service.RefreshAll()
		}()

		newPutEvent := requireEvent(t, stream)
		assert.Equal(t, "put", newPutEvent.Event())
		m.In(t).Assert(newPutEvent.Data(), m.JSONStrEqual(expectedPutData(newData)))
	})
}

func requireEvent(t *testing.T, stream *eventsource.Stream) eventsource.Event {
	return helpers.RequireValueWithMessage(t, stream.Events, time.Second*5, "timed out waiting for event")
}

func expectedPutData(sdkData SDKData) string {
	return ldvalue.ObjectBuild().Set("data", ldvalue.Raw(sdkData.Serialize())).Build().JSONString()
}
