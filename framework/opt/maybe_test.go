package opt

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type MyStruct struct {
	Prop string `json:"prop"`
}

func TestNone(t *testing.T) {
	assert.False(t, None[string]().IsDefined())

	assert.Equal(t, 0, None[int]().Value())
	assert.Equal(t, "", None[string]().Value())
	assert.Nil(t, None[*string]().Value())
	assert.Equal(t, MyStruct{}, None[MyStruct]().Value())
}

func TestSome(t *testing.T) {
	assert.True(t, Some("").IsDefined())

	assert.Equal(t, 1, Some(1).Value())
	assert.Equal(t, "x", Some("x").Value())

}

func TestOrElse(t *testing.T) {
	assert.Equal(t, 3, None[int]().OrElse(3))
	assert.Equal(t, 4, Some(4).OrElse(3))
}

func TestFromPtr(t *testing.T) {
	assert.Equal(t, None[string](), FromPtr((*string)(nil)))

	s := "x"
	assert.Equal(t, Some(s), FromPtr(&s))
}

func TestAsPtr(t *testing.T) {
	assert.Nil(t, None[int]().AsPtr())

	s := "x"
	assert.Equal(t, &s, Some(s).AsPtr())
}

func TestMarshalUnmarshal(t *testing.T) {
	testMarshalUnmarshal(t, None[int](), "null")
	testMarshalUnmarshal(t, Some(3), "3")
	testMarshalUnmarshal(t, Some(MyStruct{Prop: "x"}), `{"prop": "x"}`)

	var ms Maybe[MyStruct]
	assert.Error(t, ms.UnmarshalJSON([]byte(`malformed json`)))
	assert.Error(t, ms.UnmarshalJSON([]byte(`{"prop": true}`)))
}

func testMarshalUnmarshal[V any](t *testing.T, expected Maybe[V], expectedJSON string) {
	data, err := json.Marshal(expected)
	require.NoError(t, err)
	assert.JSONEq(t, expectedJSON, string(data))

	var actual Maybe[V]
	require.NoError(t, json.Unmarshal([]byte(expectedJSON), &actual))
	assert.Equal(t, expected, actual)
}
