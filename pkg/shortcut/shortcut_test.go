package shortcut

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShortcut(t *testing.T) {
	link := "https://alice-drive.cozy.example/"
	buf := Generate(link)
	res, err := Parse(bytes.NewReader(buf))
	assert.NoError(t, err)
	assert.Equal(t, link, res.URL)
}

func TestShortcutUnixFileFormat(t *testing.T) {
	link := "https://alice-drive.cozy.example/"
	buf := Generate(link)
	buf = bytes.ReplaceAll(buf, []byte{'\r'}, []byte{})
	res, err := Parse(bytes.NewReader(buf))
	assert.NoError(t, err)
	assert.Equal(t, link, res.URL)
}
