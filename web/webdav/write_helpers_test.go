package webdav

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsInTrash(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"exact trash dir", "/.cozy_trash", true},
		{"file inside trash", "/.cozy_trash/foo", true},
		{"nested inside trash", "/.cozy_trash/a/b/c", true},
		{"normal file", "/documents/foo", false},
		{"root", "/", false},
		{"similar prefix", "/.cozy_trash_not", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isInTrash(tt.path)
			assert.Equal(t, tt.want, got, "isInTrash(%q)", tt.path)
		})
	}
}

func TestParseDestination_ValidAbsoluteURL(t *testing.T) {
	r, _ := http.NewRequest("MOVE", "/dav/files/source.txt", nil)
	r.Header.Set("Destination", "http://localhost/dav/files/target.txt")

	got, err := parseDestination(r)
	require.NoError(t, err)
	assert.Equal(t, "/target.txt", got)
}

func TestParseDestination_ValidRelativeURL(t *testing.T) {
	r, _ := http.NewRequest("MOVE", "/dav/files/source.txt", nil)
	r.Header.Set("Destination", "/dav/files/target.txt")

	got, err := parseDestination(r)
	require.NoError(t, err)
	assert.Equal(t, "/target.txt", got)
}

func TestParseDestination_MissingHeader(t *testing.T) {
	r, _ := http.NewRequest("MOVE", "/dav/files/source.txt", nil)
	// No Destination header set.

	_, err := parseDestination(r)
	assert.Error(t, err)
}

func TestParseDestination_WrongPrefix(t *testing.T) {
	r, _ := http.NewRequest("MOVE", "/dav/files/source.txt", nil)
	r.Header.Set("Destination", "http://localhost/wrong/path")

	_, err := parseDestination(r)
	assert.Error(t, err)
}

func TestParseDestination_TraversalInDest(t *testing.T) {
	r, _ := http.NewRequest("MOVE", "/dav/files/source.txt", nil)
	r.Header.Set("Destination", "http://localhost/dav/files/../../../etc/passwd")

	_, err := parseDestination(r)
	assert.Error(t, err)
}

func TestParseDestination_URLDecoded(t *testing.T) {
	r, _ := http.NewRequest("MOVE", "/dav/files/source.txt", nil)
	r.Header.Set("Destination", "http://localhost/dav/files/my%20file.txt")

	got, err := parseDestination(r)
	require.NoError(t, err)
	assert.Equal(t, "/my file.txt", got)
}
