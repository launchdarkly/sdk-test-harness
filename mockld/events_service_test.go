package mockld

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventsServiceClosesConnectionForNextPosts(t *testing.T) {
	testLog := ldlogtest.NewMockLog()
	defer testLog.DumpIfTestFailed(t)
	service := NewEventsService(ServerSideSDK, testLog.Loggers.ForLevel(ldlog.Debug), false)
	service.SetConnectionClosedForNextPosts(2)

	httphelpers.WithServer(service, func(server *httptest.Server) {
		post := func() (*http.Response, error) {
			return http.Post(server.URL+"/bulk", "application/json", strings.NewReader(`[{"kind":"custom"}]`))
		}

		// The first two posts get no response.
		for i := 0; i < 2; i++ {
			resp, err := post()
			if err == nil {
				_ = resp.Body.Close()
			}
			assert.Error(t, err, "post %d", i+1)
		}
		_, received := service.AwaitAnalyticsEventPayload(time.Millisecond * 100)
		assert.False(t, received, "events from a post with a closed connection")

		// The next post is accepted, and its events are delivered.
		resp, err := post()
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusAccepted, resp.StatusCode)
		events, received := service.AwaitAnalyticsEventPayload(time.Second)
		require.True(t, received)
		assert.Len(t, events, 1)
	})
}
