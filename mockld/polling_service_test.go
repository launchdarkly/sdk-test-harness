package mockld

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/stretchr/testify/require"
)

func TestPollingServiceServerSide(t *testing.T) {
	doPollingServiceTests(
		t,
		ServerSideSDK,
		EmptyServerSDKData(),
		NewServerSDKDataBuilder().RawFlag("flag1", json.RawMessage(`{"key": "flag1"}`)).Build(),
		"GET",
		"/sdk/latest-all",
	)
}

func TestPollingServiceMobile(t *testing.T) {
	for _, oldUserPaths := range []bool{false, true} {
		userOrContext := h.IfElse(oldUserPaths, "user", "context")
		t.Run(userOrContext, func(t *testing.T) {
			for _, useReport := range []bool{true, false} {
				method := h.IfElse(useReport, "REPORT", "GET")
				endpoint := h.IfElse(
					useReport,
					fmt.Sprintf("/msdk/evalx/%s", userOrContext),
					fmt.Sprintf("/msdk/evalx/%ss/fakeuserdata", userOrContext),
				)
				t.Run(method, func(t *testing.T) {
					doPollingServiceTests(
						t,
						MobileSDK,
						EmptyClientSDKData(),
						NewClientSDKDataBuilder().FlagWithValue("flag1", 1, ldvalue.String("yes"), 0).Build(),
						method,
						endpoint,
					)
				})
			}
		})
	}
}

func TestPollingServiceJSClient(t *testing.T) {
	for _, oldUserPaths := range []bool{false, true} {
		userOrContext := h.IfElse(oldUserPaths, "user", "context")
		t.Run(userOrContext, func(t *testing.T) {
			for _, useReport := range []bool{true, false} {
				method := h.IfElse(useReport, "REPORT", "GET")
				endpoint := h.IfElse(
					useReport,
					fmt.Sprintf("/sdk/evalx/fakeid/%s", userOrContext),
					fmt.Sprintf("/sdk/evalx/fakeid/%ss/fakeuserdata", userOrContext),
				)
				t.Run(method, func(t *testing.T) {
					doPollingServiceTests(
						t,
						JSClientSDK,
						EmptyClientSDKData(),
						NewClientSDKDataBuilder().FlagWithValue("flag1", 1, ldvalue.String("yes"), 0).Build(),
						method,
						endpoint,
					)
				})
			}
		})
	}
}

func doPollingServiceTests(
	t *testing.T,
	sdkKind SDKKind,
	initialData SDKData,
	newData SDKData,
	httpMethod, urlPath string,
) {
	testLog := ldlogtest.NewMockLog()
	testLog.Loggers.SetMinLevel(ldlog.Debug)
	defer testLog.DumpIfTestFailed(t)

	service := NewPollingService(initialData, sdkKind, testLog.Loggers.ForLevel(ldlog.Debug))

	httphelpers.WithServer(service, func(server *httptest.Server) {
		req, _ := http.NewRequest(httpMethod, server.URL+urlPath, nil)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.NotNil(t, resp.Body)
		defer resp.Body.Close()

		require.Equal(t, 200, resp.StatusCode, "got error status for %s %s", req.Method, req.URL)

		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		m.In(t).Assert(data, m.JSONStrEqual(string(initialData.Serialize())))
	})
}
