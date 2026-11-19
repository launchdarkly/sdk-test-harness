package mockld

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventIsDiagnostic(t *testing.T) {
	for _, p := range []struct {
		json     string
		expected bool
	}{
		{`{"kind":"diagnostic-init"}`, true},
		{`{"kind":"diagnostic"}`, true},
		{`{"kind":"custom"}`, false},
		{`{"kind":"identify"}`, false},
		{`{"key":"no-kind"}`, false},
		{`"diagnostic"`, false},
		{`null`, false},
	} {
		t.Run(p.json, func(t *testing.T) {
			var e Event
			require.NoError(t, e.UnmarshalJSON([]byte(p.json)))
			assert.Equal(t, p.expected, e.IsDiagnostic())
		})
	}
}
