package webdav

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
