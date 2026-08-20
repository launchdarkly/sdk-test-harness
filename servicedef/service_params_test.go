package servicedef

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCapabilityBootstrapName(t *testing.T) {
	assert.Equal(t, "bootstrap", CapabilityBootstrap)
}
