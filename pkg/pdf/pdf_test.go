package pdf

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Extract_Page(t *testing.T) {
	if testing.Short() {
		t.Skipf("this test require the \"gs\" binary, skip it due to the \"--short\" flag")
	}

	service := NewService("gs")
	input, err := os.Open("../../tests/fixtures/dev-desktop.pdf")
	require.NoError(t, err)
	defer input.Close()

	extracted, err := service.ExtractPage(input, 1)
	require.NoError(t, err)

	// We cannot compare the output to an expected PDF file, as there many
	// things that change from one run to another: CreationDate, uuid, etc.
	// So, we are checking that it's a PDF, and it has the expected signature
	// from ImageMagick.
	content := extracted.Bytes()
	start := []byte("%PDF-1.7")
	require.Equal(t, start, content[:len(start)])

	expected := "5b49b84d59866b2f6d825957c55c2c2681656e5de52e30c1439fbaf197fe1d14"
	cmd := exec.Command("identify", "-quiet", "-format", "%#", "-")
	cmd.Stdin = extracted
	signature, err := cmd.Output()
	require.NoError(t, err)
	require.Equal(t, []byte(expected), signature)
}
