package vfs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddPageToName(t *testing.T) {
	name := addPageToName("driving licence.pdf", 2)
	require.Equal(t, "driving licence (2).pdf", name)
}
