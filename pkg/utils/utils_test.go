package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRandomString(t *testing.T) {
	s1 := RandomString(10)
	s2 := RandomString(20)
	assert.Len(t, s1, 10)
	assert.Len(t, s2, 20)
	assert.NotEqual(t, s1, s2)
}
