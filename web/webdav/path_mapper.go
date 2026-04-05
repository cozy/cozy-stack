package webdav

import "errors"

// ErrPathTraversal is returned by davPathToVFSPath when the input contains
// any form of path traversal (literal "..", percent-encoded variants,
// null bytes, encoded slashes, etc.). Callers use errors.Is to distinguish
// traversal rejections from generic validation failures.
//
// NOTE: compile-only stub declared in Plan 01-02 so the package's test
// binary can build. Plan 01-03 replaces davPathToVFSPath with the real
// implementation; the sentinel itself is final.
var ErrPathTraversal = errors.New("webdav: path traversal rejected")

// davPathToVFSPath converts a raw URL :path parameter into a normalised
// absolute VFS path. This is a compile-only stub — Plan 01-03 will
// replace it with the real implementation driven by path_mapper_test.go.
func davPathToVFSPath(string) (string, error) {
	return "", ErrPathTraversal
}
