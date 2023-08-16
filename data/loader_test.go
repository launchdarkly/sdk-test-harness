package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileEmbedding(t *testing.T) {
	_, err := dataFilesRoot.ReadFile("data-files/README.md")
	assert.NoError(t, err)

	files, err := dataFilesRoot.ReadDir("data-files/server-side-eval")
	assert.NoError(t, err)
	assert.NotEqual(t, 0, len(files))
}
