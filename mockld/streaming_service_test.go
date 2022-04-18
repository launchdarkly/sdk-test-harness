package mockld

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework/helpers"
	h "github.com/launchdarkly/sdk-test-harness/framework/helpers"

	"github.com/launchdarkly/eventsource"
	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlogtest"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamingServiceServerSide(t *testing.T) {
	doStreamingServiceTests(
		t,
		ServerSideSDK,
		EmptyServerSDKData(),
		NewServerSDKDataBuilder().RawFlag("flag1", json.RawMessage(`{"key": "flag1"}`)).Build(),
		"GET",
		"/all",
		expectedServerSidePutData,
	)
}

func TestStreamingServiceMobile(t *testing.T) {
	for _, useReport := range []bool{true, false} {
		method := h.IfElse(useReport, "REPORT", "GET")
		t.Run(method, func(t *testing.T) {
			doStreamingServiceTests(
				t,
				MobileSDK,
				EmptyClientSDKData(),
				NewClientSDKDataBuilder().FlagWithValue("flag1", 1, ldvalue.String("yes"), 0).Build(),
				method,
				h.IfElse(useReport, "/meval", "/meval/fakeuserdata"),
				expectedClientSidePutData,
			)
		})
	}
}

func TestStreamingServiceJSClient(t *testing.T) {
	for _, useReport := range []bool{true, false} {
		method := h.IfElse(useReport, "REPORT", "GET")
		t.Run(method, func(t *testing.T) {
			doStreamingServiceTests(
				t,
				JSClientSDK,
				EmptyClientSDKData(),
				NewClientSDKDataBuilder().FlagWithValue("flag1", 1, ldvalue.String("yes"), 0).Build(),
				method,
				h.IfElse(useReport, "/eval/fakeid", "/eval/fakeid/fakeuserdata"),
				expectedClientSidePutData,
			)
		})
	}
}

func doStreamingServiceTests(
	t *testing.T,
	sdkKind SDKKind,
	initialData SDKData,
	newData SDKData,
	httpMethod, urlPath string,
	makeExpectedPutData func(SDKData) string,
) {
	testLog := ldlogtest.NewMockLog()
	testLog.Loggers.SetMinLevel(ldlog.Debug)
	defer testLog.DumpIfTestFailed(t)

	service := NewStreamingService(initialData, sdkKind, testLog.Loggers.ForLevel(ldlog.Debug))

	httphelpers.WithServer(service, func(server *httptest.Server) {
		req, _ := http.NewRequest(httpMethod, server.URL+urlPath, nil)
		stream, err := eventsource.SubscribeWithRequest("", req)
		require.NoError(t, err)
		defer stream.Close()

		initialEvent := requireEvent(t, stream)
		assert.Equal(t, "put", initialEvent.Event())
		m.In(t).Assert(initialEvent.Data(), m.JSONStrEqual(makeExpectedPutData(initialData)))

		go func() {
			service.SetInitialData(newData)
			service.RefreshAll()
		}()

		newPutEvent := requireEvent(t, stream)
		assert.Equal(t, "put", newPutEvent.Event())
		m.In(t).Assert(newPutEvent.Data(), m.JSONStrEqual(makeExpectedPutData(newData)))
	})
}

func requireEvent(t *testing.T, stream *eventsource.Stream) eventsource.Event {
	return helpers.RequireValueWithMessage(t, stream.Events, time.Second*5, "timed out waiting for event")
}

func expectedServerSidePutData(sdkData SDKData) string {
	return ldvalue.ObjectBuild().Set("data", ldvalue.Raw(sdkData.Serialize())).Build().JSONString()
}

func expectedClientSidePutData(sdkData SDKData) string {
	return string(sdkData.Serialize())
}
