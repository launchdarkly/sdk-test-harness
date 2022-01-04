package ldtest

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

type regexFilterTestParams struct {
	run         []string
	skip        []string
	testID      TestID
	shouldMatch bool
}

func TestRegexFilters(t *testing.T) {
	allParams := []regexFilterTestParams{
		// matches everything by default
		{nil, nil, TestID(nil), true},
		{nil, nil, TestID{"a"}, true},
		{nil, nil, TestID{"a", "b"}, true},

		// --run with single component
		{[]string{"a"}, nil, TestID(nil), true},
		{[]string{"a"}, nil, TestID{"a"}, true},
		{[]string{"a"}, nil, TestID{"b"}, false},
		{[]string{"a"}, nil, TestID{"xax"}, true},
		{[]string{"a"}, nil, TestID{"a", "b"}, true},

		// --run with multiple components
		{[]string{"a/b"}, nil, TestID(nil), true},
		{[]string{"a/b"}, nil, TestID{"a"}, true},
		{[]string{"a/b"}, nil, TestID{"b"}, false},
		{[]string{"a/b"}, nil, TestID{"a", "b"}, true},
		{[]string{"a/b"}, nil, TestID{"xax", "xbx"}, true},

		// --run with multiple patterns
		{[]string{"a", "b"}, nil, TestID(nil), true},
		{[]string{"a", "b"}, nil, TestID{"a"}, true},
		{[]string{"a", "b"}, nil, TestID{"b"}, true},
		{[]string{"a", "b"}, nil, TestID{"c"}, false},
		{[]string{"a", "b"}, nil, TestID{"a", "c"}, true},
		{[]string{"a", "b"}, nil, TestID{"b", "c"}, true},
		{[]string{"a", "b"}, nil, TestID{"xax", "xbx"}, true},

		// --skip with single component
		{nil, []string{"a"}, TestID(nil), true},
		{nil, []string{"a"}, TestID{"a"}, false},
		{nil, []string{"a"}, TestID{"b"}, true},
		{nil, []string{"a"}, TestID{"xax"}, false},
		{nil, []string{"a"}, TestID{"a", "b"}, false},

		// --skip with multiple components
		{nil, []string{"a/b"}, TestID(nil), true},
		{nil, []string{"a/b"}, TestID{"a"}, true},
		{nil, []string{"a/b"}, TestID{"b"}, true},
		{nil, []string{"a/b"}, TestID{"a", "b"}, false},
		{nil, []string{"a/b"}, TestID{"a", "b", "c"}, false},
		{nil, []string{"a/b"}, TestID{"a", "c"}, true},
		{nil, []string{"a/b"}, TestID{"xax", "xbx"}, false},

		// --skip with multiple patterns
		{nil, []string{"a", "b"}, TestID(nil), true},
		{nil, []string{"a", "b"}, TestID{"a"}, false},
		{nil, []string{"a", "b"}, TestID{"b"}, false},
		{nil, []string{"a", "b"}, TestID{"c"}, true},
		{nil, []string{"a", "b"}, TestID{"a", "c"}, false},
		{nil, []string{"a", "b"}, TestID{"b", "c"}, false},
		{nil, []string{"a", "b"}, TestID{"xax", "c"}, false},
		{nil, []string{"a", "b"}, TestID{"c", "a"}, true},

		// --skip overrides --run
		{[]string{"y"}, []string{"n"}, TestID{"y"}, true},
		{[]string{"y"}, []string{"n"}, TestID{"yn"}, false},
	}
	for _, params := range allParams {
		var r RegexFilters
		for _, s := range params.run {
			r.MustMatch.Set(s)
		}
		for _, s := range params.skip {
			r.MustNotMatch.Set(s)
		}
		t.Run(fmt.Sprintf("run=%s, skip=%s, id=%s", r.MustMatch, r.MustNotMatch, params.testID), func(t *testing.T) {
			assert.Equal(t, params.shouldMatch, r.Match(params.testID))
		})
	}
}
