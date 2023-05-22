package avatar

import (
	"bytes"
	"context"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Initials_PNG(t *testing.T) {
	if testing.Short() {
		t.Skipf("this test require the \"convert\" binary, skip it due to the \"--short\" flag")
	}

	client, err := NewPNGInitials("convert")
	require.NoError(t, err)
	defer client.Shutdown(context.Background())

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

func Test_Initials_PNG_Shutdown(t *testing.T) {
	if testing.Short() {
		t.Skipf("this test require the \"convert\" binary, skip it due to the \"--short\" flag")
	}

	client, err := NewPNGInitials("convert")
	require.NoError(t, err)

	var rawRes []byte
	generateFinished := false
	isStarted := make(chan struct{}, 1)
	go func() {
		isStarted <- struct{}{}
		rawRes, err = client.Generate(context.Background(), "JD", "#FF7F1B")
		generateFinished = true
	}()

	// Ensure that the Generate is started and not finished before calling
	// `Shutdown`
	<-isStarted
	time.Sleep(2 * time.Millisecond)
	require.False(t, client.dirLock.TryLock())
	require.False(t, generateFinished)

	// The shut
	client.Shutdown(context.Background())
	assert.True(t, generateFinished)
	assert.NotEmpty(t, rawRes)
	assert.NoError(t, err)
}
