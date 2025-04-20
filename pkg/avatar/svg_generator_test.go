package avatar

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Initials_SVG(t *testing.T) {
	svc := NewService(nil, "")
	data, contentType, err := svc.GenerateInitials("")
	require.NoError(t, err)

	rawExpected, err := os.ReadFile("./testdata/anonymous.svg")
	require.NoError(t, err)

	require.Equal(t, contentType, "image/svg+xml")
	require.Equal(t, data, rawExpected, "images doesn't have the same size")
}
