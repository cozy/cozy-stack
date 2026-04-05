package webdav

import (
	"errors"
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
// Echo has already URL-decoded the wildcard once, so any remaining %XX
// sequence is a double-encoding attempt and is rejected outright.
//
// The function performs exactly these steps:
//  1. reject null bytes
//  2. reject residual %2e / %2f sequences (case-insensitive) — these catch
//     both double-encoded traversal (%252e → %2e after Echo decode) and
//     encoded slashes smuggling segment separators past path.Clean
//  3. path.Clean("/files/" + input) — resolves "..", removes double slashes
//     and trailing slashes
//  4. assert the cleaned path is either "/files" or has "/files/" prefix,
//     which traps any ".." that escapes the scope
//  5. strip the "/files" prefix to yield the VFS path, substituting "/"
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

	// The WebDAV URL space is rooted at /files — we prepend "/files" before
	// cleaning to re-anchor the wildcard for the prefix check below.
	cleaned := path.Clean("/files/" + rawParam)

	if cleaned != "/files" && !strings.HasPrefix(cleaned, "/files/") {
		return "", ErrPathTraversal
	}

	vfsPath := strings.TrimPrefix(cleaned, "/files")
	if vfsPath == "" {
		vfsPath = "/"
	}
	return vfsPath, nil
}

// containsEncodedTraversal reports whether s carries a still-encoded percent
// escape. Echo decodes the URL wildcard once before it reaches us, so any
// surviving '%' is either a double encoding (e.g. "%252e%252e" → "%2e%2e" after
// one decode) or an attempt to smuggle a dot/slash past path.Clean. We reject
// every residual percent sequence rather than enumerating %2e / %2f, which
// catches the double-encoded variant on the very first decode pass.
func containsEncodedTraversal(s string) bool {
	return strings.ContainsRune(s, '%')
}
