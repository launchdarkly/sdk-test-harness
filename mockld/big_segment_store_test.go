package mockld

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/require"
)

func TestMockBigSegmentStoreMembershipResponseShapes(t *testing.T) {
	testLog := ldlogtest.NewMockLog()
	testLog.Loggers.SetMinLevel(ldlog.Debug)
	defer testLog.DumpIfTestFailed(t)

	for _, p := range []struct {
		name       string
		membership map[string]bool
		found      bool
		expected   string
	}{
		{"context not found", nil, false, `{}`},
		{"null membership", nil, true, `{"values": null}`},
		{"empty membership", map[string]bool{}, true, `{"values": {}}`},
		{"membership with values", map[string]bool{"a": true, "b": false}, true, `{"values": {"a": true, "b": false}}`},
	} {
		t.Run(p.name, func(t *testing.T) {
			service := NewMockBigSegmentStoreService(
				func() (ldtime.UnixMillisecondTime, error) { return 0, nil },
				func(string) (map[string]bool, bool, error) { return p.membership, p.found, nil },
				testLog.Loggers.ForLevel(ldlog.Debug),
			)
			rec := httptest.NewRecorder()
			service.ServeHTTP(rec, httptest.NewRequest("POST", "/getMembership",
				strings.NewReader(`{"contextHash": "x"}`)))
			require.Equal(t, http.StatusOK, rec.Code)
			m.In(t).Assert(rec.Body.Bytes(), m.JSONStrEqual(p.expected))
		})
	}
}
