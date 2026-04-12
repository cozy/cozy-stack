package webdav

import (
	"errors"
	"net/url"
	"path"
	"strings"
)

// ErrPathTraversal is returned by davPathToVFSPath when the input contains
// any form of path traversal (literal "..", percent-encoded variants,
// null bytes, encoded slashes, etc.). Callers use errors.Is to distinguish
// traversal rejections from generic validation failures.
var ErrPathTraversal = errors.New("webdav: path traversal rejected")

// davPathToVFSPath converts a WebDAV URL wildcard (as returned by
// echo.Context.Param("*")) into a normalised absolute VFS path rooted at "/".
//
// Echo does NOT URL-decode the wildcard before handing it to the handler, so
// the raw parameter may contain percent-encoded sequences. We handle them as
// follows:
//  1. reject literal null bytes
//  2. reject dangerous percent-encoded sequences that path.Clean cannot
//     neutralise: %2e/%2E (dot), %2f/%2F (slash), %00 (null) — these catch
//     double-encoded traversal (%252e → %2e) and encoded-slash smuggling
//  3. URL-decode the remaining percent-encoded sequences (valid UTF-8 filenames
//     such as %e2%82%ac → €) via url.PathUnescape
//  4. reject null bytes in the decoded result (belt-and-suspenders)
//  5. path.Clean("/files/" + decoded) — resolves "..", removes double slashes
//     and trailing slashes
//  6. assert the cleaned path is either "/files" or has "/files/" prefix,
//     which traps any ".." that escapes the scope
//  7. strip the "/files" prefix to yield the VFS path, substituting "/"
//     when the result would be empty
//
// Every failure returns ErrPathTraversal so callers can log and respond
// uniformly.
func davPathToVFSPath(rawParam string) (string, error) {
	if strings.ContainsRune(rawParam, 0) {
		return "", ErrPathTraversal
	}
	if containsEncodedTraversal(rawParam) {
		return "", ErrPathTraversal
	}

	// Decode remaining percent-encoded sequences (e.g. %e2%82%ac → €).
	// url.PathUnescape decodes %XX but preserves literal '/' unlike QueryUnescape.
	decoded, err := url.PathUnescape(rawParam)
	if err != nil {
		// Malformed percent-encoding (e.g. truncated %X).
		return "", ErrPathTraversal
	}
	// Post-decode traversal check: double-encoding like %252e decodes to %2e.
	// We must re-check after decoding to catch this pattern.
	if containsEncodedTraversal(decoded) {
		return "", ErrPathTraversal
	}
	// Post-decode null byte check — belt-and-suspenders in case %00 slipped through.
	if strings.ContainsRune(decoded, 0) {
		return "", ErrPathTraversal
	}

	// The WebDAV URL space is rooted at /files — we prepend "/files" before
	// cleaning to re-anchor the wildcard for the prefix check below.
	cleaned := path.Clean("/files/" + decoded)

	if cleaned != "/files" && !strings.HasPrefix(cleaned, "/files/") {
		return "", ErrPathTraversal
	}

	vfsPath := strings.TrimPrefix(cleaned, "/files")
	if vfsPath == "" {
		vfsPath = "/"
	}
	return vfsPath, nil
}

// containsEncodedTraversal reports whether s contains a percent-encoded
// sequence that could be used for path traversal or null-byte injection.
// We target the specific dangerous sequences rather than rejecting all '%':
//   - %2e / %2E  — encoded dot (used in ../.. traversal)
//   - %2f / %2F  — encoded slash (smuggles a path separator past path.Clean)
//   - %00        — encoded null byte
//
// Other percent-encoded sequences (e.g. %e2%82%ac for the euro sign) are
// legitimate UTF-8 filenames and must be allowed.
//
// Note: Echo does NOT decode the wildcard parameter before handing it to the
// handler, so percent-encoded sequences reach this function as-is.
func containsEncodedTraversal(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "%2e") ||
		strings.Contains(lower, "%2f") ||
		strings.Contains(lower, "%00")
}
