package avatar

import (
	"bytes"
	"context"
	"image/png"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitialsPNG(t *testing.T) {
	if testing.Short() {
		t.Skipf("this test require the \"convert\" binary, skip it due to the \"--short\" flag")
	}

	client, err := NewPNGInitials("convert")
	require.NoError(t, err)

	ctx := context.Background()

	rawRes, err := client.Generate(ctx, "JD", "#FF7F1B")
	require.NoError(t, err)

	rawExpected, err := os.ReadFile("./testdata/initials-convert.png")
	require.NoError(t, err)

	// Due to the compression algorithm we can't compare the bytes
	// as they change for each generation. The only solution is to decode
	// the image and check pixel by pixel.
	// This also allow to ensure that the end result is exactly the same.
	resImg, err := png.Decode(bytes.NewReader(rawRes))
	require.NoError(t, err)

	expectImg, err := png.Decode(bytes.NewReader(rawExpected))
	require.NoError(t, err)

	require.Equal(t, expectImg.Bounds(), resImg.Bounds(), "images doesn't have the same size")
}
