package mockld

import (
	"io"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrokenBodyHandlers(t *testing.T) {
	body := []byte(`{"flags": {"flag1": {"key": "flag1", "version": 1}}, "segments": {}}`)
	for name, p := range map[string]struct {
		handler       http.Handler
		expectedError error
	}{
		"truncated": {TruncatedBodyHandler("application/json", body), io.ErrUnexpectedEOF},
		"reset":     {ResetMidBodyHandler("application/json", body), syscall.ECONNRESET},
	} {
		t.Run(name, func(t *testing.T) {
			httphelpers.WithServer(p.handler, func(server *httptest.Server) {
				client := http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
				resp, err := client.Get(server.URL)
				require.NoError(t, err)
				defer resp.Body.Close()
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				assert.Equal(t, int64(len(body)), resp.ContentLength)

				data, err := io.ReadAll(resp.Body)
				assert.ErrorIs(t, err, p.expectedError, "reading the body should fail")
				assert.Equal(t, string(body[:len(body)/2]), string(data))
			})
		})
	}
}
