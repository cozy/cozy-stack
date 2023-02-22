package revision

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGeneration(t *testing.T) {
	assert.Equal(t, 1, Generation("1-aaa"))
	assert.Equal(t, 3, Generation("3-123"))
	assert.Equal(t, 10, Generation("10-1f2"))
}
