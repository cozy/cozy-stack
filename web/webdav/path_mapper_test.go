package webdav

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDavPathToVFSPath exercises davPathToVFSPath — the unexported helper that
// converts a raw :path URL parameter into a normalised absolute VFS path, and
// rejects any form of path traversal before it reaches the VFS layer.
//
// Covers TEST-02, ROUTE-03, ROUTE-05, SEC-02.
func TestDavPathToVFSPath(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantPath  string
		wantError bool
	}{
		// Valid inputs
		{name: "root empty", input: "", wantPath: "/"},
		{name: "root slash", input: "/", wantPath: "/"},
		{name: "simple", input: "Documents", wantPath: "/Documents"},
		{name: "nested", input: "Documents/report.docx", wantPath: "/Documents/report.docx"},
		{name: "trailing slash", input: "Documents/", wantPath: "/Documents"},
		{name: "unicode", input: "Documents/répertoire", wantPath: "/Documents/répertoire"},

		// Percent-encoded UTF-8 filenames — must be ALLOWED (litmus put_get_utf8_segment)
		{name: "euro sign percent-encoded", input: "litmus/res-%e2%82%ac", wantPath: "/litmus/res-€"},
		{name: "percent-encoded ascii letter", input: "litmus/res-%41", wantPath: "/litmus/res-A"},
		{name: "mixed utf8 percent-encoded", input: "Documents/%c3%a9t%c3%a9", wantPath: "/Documents/été"},

		// Traversal / malicious inputs — must be rejected with ErrPathTraversal
		{name: "dotdot literal", input: "../etc/passwd", wantError: true},
		{name: "encoded dotdot lowercase", input: "%2e%2e/etc", wantError: true},
		{name: "encoded dotdot uppercase", input: "%2E%2E/etc", wantError: true},
		{name: "double encoded", input: "%252e%252e/etc", wantError: true},
		{name: "null byte", input: "Documents\x00evil", wantError: true},
		{name: "encoded slash", input: "Documents%2fsecret", wantError: true},
		{name: "encoded null byte", input: "Documents%00evil", wantError: true},
		{name: "settings prefix rejected", input: "../settings", wantError: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := davPathToVFSPath(tc.input)
			if tc.wantError {
				require.Error(t, err, "expected error for input %q", tc.input)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantPath, got)
		})
	}
}

// TestDavPathToVFSPath_SentinelError verifies that traversal rejections return
// an error that can be compared against the exported ErrPathTraversal sentinel
// using errors.Is. This lets callers distinguish traversal errors from generic
// validation failures.
func TestDavPathToVFSPath_SentinelError(t *testing.T) {
	_, err := davPathToVFSPath("../etc/passwd")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPathTraversal),
		"expected ErrPathTraversal, got %v", err)
}
