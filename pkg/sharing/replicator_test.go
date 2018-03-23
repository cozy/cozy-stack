package sharing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRevGeneration(t *testing.T) {
	assert.Equal(t, 1, RevGeneration("1-aaa"))
	assert.Equal(t, 3, RevGeneration("3-123"))
	assert.Equal(t, 10, RevGeneration("10-1f2"))
}

func TestComputePossibleAncestors(t *testing.T) {
	wants := []string{"2-b"}
	haves := []string{"1-a", "2-a", "3-a"}
	pas := computePossibleAncestors(wants, haves)
	expected := []string{"1-a"}
	assert.Equal(t, expected, pas)

	wants = []string{"2-b", "2-c", "4-b"}
	haves = []string{"1-a", "2-a", "3-a", "4-a"}
	pas = computePossibleAncestors(wants, haves)
	expected = []string{"1-a", "3-a"}
	assert.Equal(t, expected, pas)

	wants = []string{"5-b"}
	haves = []string{"1-a", "2-a", "3-a"}
	pas = computePossibleAncestors(wants, haves)
	expected = []string{"3-a"}
	assert.Equal(t, expected, pas)
}
