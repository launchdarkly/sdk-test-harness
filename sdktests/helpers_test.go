package sdktests

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFormatList(t *testing.T) {
	t.Run("array/slice arguments", func(t *testing.T) {
		assert.Equal(t, "a, b, c", formatSlice([]string{"a", "b", "c"}))
		assert.Equal(t, "just a", formatSlice([]string{"just a"}))
		assert.Equal(t, "a, b", formatSlice([]string{"a, b"}))
		assert.Equal(t, "", formatSlice([]string{}))

		type stringAlias string
		assert.Equal(t, "a, b, c", formatSlice([]stringAlias{"a", "b", "c"}))

		type intAlias int
		assert.Equal(t, "1, 2, 3", formatSlice([]intAlias{1, 2, 3}))
	})

	t.Run("non array/slice argument", func(t *testing.T) {
		defer func() {
			if err := recover(); err == nil {
				t.Fatal("expected panic")
			}
		}()
		formatSlice("not a slice")
	})
}
