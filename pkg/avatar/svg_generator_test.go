package avatar

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Initials_SVG(t *testing.T) {
	svc := NewService(nil, "")
	for _, test := range []struct{ testname, filename, initials string }{
		{"AnonymousSVG", "./testdata/anonymous.svg", ""},
		{"WWInitialsSVG", "./testdata/ww.svg", "Winston Wombat"},
	} {
		t.Run(test.testname, func(t *testing.T) {
			data, contentType, err := svc.GenerateInitials(test.initials, func(familyName, style, weight string) ([]byte, error) {
				return []byte("invalid test font data"), nil
			}, EmbedFont)
			require.NoError(t, err)

			rawExpected, err := os.ReadFile(test.filename)
			require.NoError(t, err)

			require.Equal(t, contentType, "image/svg+xml")
			require.Equal(t, string(data), string(rawExpected), "images don't have the same data")
		})
	}
}
