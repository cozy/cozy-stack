package sharing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanFilename(t *testing.T) {
	cases := map[string]string{
		"foo":             "foo",
		"invalid <chars>": "invalid -chars-",
	}
	for filename, expected := range cases {
		t.Run(filename, func(t *testing.T) {
			assert.Equal(t, expected, cleanFilename(filename))
		})
	}
}
