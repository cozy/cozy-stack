// Package webdav implements an RFC 4918 WebDAV server exposing the Cozy
// instance /files/ tree. See .planning/phases/01-foundation/ for design.
package webdav

import "net/http"

// webdavMethods lists every HTTP method the WebDAV handlers will accept.
// OPTIONS is first so it can be registered separately (bypasses auth).
var webdavMethods = []string{
	http.MethodOptions,
	"PROPFIND",
	http.MethodGet,
	http.MethodHead,
	http.MethodPut,
	http.MethodDelete,
	"MKCOL",
	"COPY",
	"MOVE",
}

// Routes will be implemented in a later wave.
// Stub kept unexported-ish until wave 3.
var _ = webdavMethods
