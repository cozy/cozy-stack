package avatar

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSVgoInitials_ok(t *testing.T) {
	client := NewSVGoInitials()

	ctx := context.Background()

	rawRes, err := client.Generate(ctx, "JD", "#FF7F1B")
	require.NoError(t, err)

	rawExpected, err := os.ReadFile("./testdata/initials-svgo.svg")
	require.NoError(t, err)

	assert.Equal(t, rawExpected, rawRes)
}

func TestSVgoInitials_returns_SVG_content_type(t *testing.T) {
	client := NewSVGoInitials()

	assert.Equal(t, "image/svg+xml", client.ContentType())
}

func TestSVgoInitials_with_invalid_color(t *testing.T) {
	client := NewSVGoInitials()

	ctx := context.Background()

	rawRes, err := client.Generate(ctx, "JD", "invalid-color")
	assert.Nil(t, rawRes)
	assert.EqualError(t, err, "failed to parse the color: encoding/hex: invalid byte: U+0069 'i'")
}
