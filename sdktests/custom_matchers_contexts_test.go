package sdktests

import (
	"encoding/json"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/stretchr/testify/require"
)

func TestJSONMatchesContext(t *testing.T) {
	type testParams struct {
		c     ldcontext.Context
		input string
	}

	t.Run("match", func(t *testing.T) {
		for _, p := range []testParams{
			{
				c:     ldcontext.NewWithKind("a", "b"),
				input: `{"kind": "a", "key": "b"}`,
			},
			{
				c:     ldcontext.NewWithKind("a", "b"),
				input: `{"kind": "a", "key": "b", "anonymous": false}`,
			},
			{
				c:     ldcontext.NewWithKind("a", "b"),
				input: `{"kind": "a", "key": "b", "_meta": {}}`,
			},
			{
				c:     ldcontext.NewWithKind("a", "b"),
				input: `{"kind": "a", "key": "b", "_meta": {"secondary": null}}`,
			},
			{
				c:     ldcontext.NewWithKind("a", "b"),
				input: `{"kind": "a", "key": "b", "_meta": {"privateAttributes": null}}`,
			},
			{
				c:     ldcontext.NewWithKind("a", "b"),
				input: `{"kind": "a", "key": "b", "_meta": {"privateAttributes": []}}`,
			},
			{
				c:     ldcontext.NewBuilder("a").Anonymous(true).Name("b").Secondary("c").Build(),
				input: `{"kind": "user", "key": "a", "anonymous": true, "name": "b", "_meta": {"secondary": "c"}}`,
			},
			{
				c:     ldcontext.NewBuilder("a").Name("b").Private("d", "c").Build(),
				input: `{"kind": "user", "key": "a", "name": "b", "_meta": {"privateAttributes": ["c", "d"]}}`,
			},
			{
				c: ldcontext.NewMulti(
					ldcontext.NewWithKind("kind1", "key1"), ldcontext.NewWithKind("kind2", "key2"),
				),
				input: `{"kind": "multi", "kind1": {"key": "key1"}, "kind2": {"key": "key2"}}`,
			},
		} {
			t.Run(p.input, func(t *testing.T) {
				m.In(t).Assert(json.RawMessage(p.input), JSONMatchesContext(p.c))
			})
		}
	})

	t.Run("non-match", func(t *testing.T) {
		for _, p := range []testParams{
			{
				c:     ldcontext.NewWithKind("a", "b"),
				input: `{"kind": "c", "key": "b"}`,
			},
			{
				c:     ldcontext.NewWithKind("a", "b"),
				input: `{"kind": "a", "key": "c"}`,
			},
			{
				c:     ldcontext.NewWithKind("a", "b"),
				input: `{"kind": "a", "key": "b", "name": "c"}`,
			},
			{
				c:     ldcontext.NewWithKind("a", "b"),
				input: `{"kind": "a", "key": "b", "attr1": "c"}`,
			},
			{
				c:     ldcontext.NewWithKind("a", "b"),
				input: `{"kind": "a", "key": "b", "anonymous": true}`,
			},
			{
				c:     ldcontext.NewWithKind("a", "b"),
				input: `{"kind": "a", "key": "b", "_meta": {"secondary": "c"}}`,
			},
			{
				c:     ldcontext.NewWithKind("a", "b"),
				input: `{"kind": "a", "key": "b", "_meta": {"privateAttributes": ["c", "d"]}}`,
			},
			{
				c:     ldcontext.NewBuilder("a").Anonymous(true).Build(),
				input: `{"kind": "user", "key": "a"}`,
			},
			{
				c:     ldcontext.NewBuilder("a").Anonymous(true).Build(),
				input: `{"kind": "user", "key": "a", "anonymous": false}`,
			},
			{
				c:     ldcontext.NewBuilder("a").Secondary("b").Build(),
				input: `{"kind": "user", "key": "a", "anonymous": true, "name": "b", "_meta": {"secondary": "c"}}`,
			},
			{
				c:     ldcontext.NewBuilder("a").Name("b").Private("c", "d", "e").Build(),
				input: `{"kind": "user", "key": "a", "name": "b", "_meta": {"privateAttributes": ["c", "d"]}}`,
			},
			{
				c: ldcontext.NewMulti(
					ldcontext.NewWithKind("kind1", "key1"), ldcontext.NewWithKind("kind2", "key3"),
				),
				input: `{"kind": "multi", "kind1": {"key": "key1"}, "kind2": {"key": "key2"}}`,
			},
			{
				c: ldcontext.NewMulti(
					ldcontext.NewWithKind("kind1", "key1"), ldcontext.NewWithKind("kind2", "key2"),
				),
				input: `{"kind": "multi", "kind1": {"key": "key1"}, "kind2": {"key": "key2"}, "kind3": {"key": "key3"}}`,
			},
			{
				c: ldcontext.NewMulti(
					ldcontext.NewWithKind("kind1", "key1"),
					ldcontext.NewWithKind("kind2", "key2"),
					ldcontext.NewWithKind("kind3", "key3"),
				),
				input: `{"kind": "multi", "kind1": {"key": "key1"}, "kind2": {"key": "key2"}}`,
			},
		} {
			t.Run(p.input, func(t *testing.T) {
				var parsed interface{}
				require.NoError(t, json.Unmarshal([]byte(p.input), &parsed))
				if pass, _ := JSONMatchesContext(p.c).Test(json.RawMessage(p.input)); pass {
					t.Errorf("context %s should not have matched, but did", p.c)
				}
			})
		}
	})
}

func TestJSONMatchesEventContext(t *testing.T) {
	// Here we're trusting that the logic is all the same except for the redacted attributes

	type testParams struct {
		c                ldcontext.Context
		input            string
		redactedShouldBe []string
	}

	t.Run("match", func(t *testing.T) {
		for _, p := range []testParams{
			{
				c:     ldcontext.NewWithKind("a", "b"),
				input: `{"kind": "a", "key": "b"}`,
			},
			{
				c:     ldcontext.NewWithKind("a", "b"),
				input: `{"kind": "a", "key": "b", "_meta": {"redactedAttributes": null}}`,
			},
			{
				c:     ldcontext.NewWithKind("a", "b"),
				input: `{"kind": "a", "key": "b", "_meta": {"redactedAttributes": []}}`,
			},
			{
				c:                ldcontext.New("a"),
				input:            `{"kind": "user", "key": "a", "_meta": {"redactedAttributes": ["b", "c"]}}`,
				redactedShouldBe: []string{"c", "b"},
			},
		} {
			t.Run(p.input, func(t *testing.T) {
				m.In(t).Assert(json.RawMessage(p.input), JSONMatchesEventContext(p.c, p.redactedShouldBe))
			})
		}
	})

	t.Run("non-match", func(t *testing.T) {
		for _, p := range []testParams{
			{
				c:                ldcontext.New("a"),
				input:            `{"kind": "user", "key": "a"}`,
				redactedShouldBe: []string{"b"},
			},
			{
				c:                ldcontext.New("a"),
				input:            `{"kind": "user", "key": "a", "_meta": {"redactedAttributes": ["b", "c"]}}`,
				redactedShouldBe: []string{"b"},
			},
			{
				c:                ldcontext.NewBuilder("a").Private("b").Build(),
				input:            `{"kind": "user", "key": "a", "_meta": {"privateAttributes": ["b"]}}`,
				redactedShouldBe: []string{"b"},
			},
		} {
			t.Run(p.input, func(t *testing.T) {
				var parsed interface{}
				require.NoError(t, json.Unmarshal([]byte(p.input), &parsed))
				if pass, _ := JSONMatchesEventContext(p.c, p.redactedShouldBe).Test(json.RawMessage(p.input)); pass {
					t.Errorf("context %s should not have matched (wanted redacted attributes: %v), but did", p.c, p.redactedShouldBe)
				}
			})
		}
	})
}
