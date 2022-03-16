package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileEmbedding(t *testing.T) {
	_, err := dataFilesRoot.ReadFile("files/README.md")
	assert.NoError(t, err)

	files, err := dataFilesRoot.ReadDir("files/server-side-eval")
	assert.NoError(t, err)
	assert.NotEqual(t, 0, len(files))
}
