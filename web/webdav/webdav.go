// Package webdav implements an RFC 4918 Class 1 WebDAV server exposing
// the Cozy instance /files/ tree. See .planning/phases/01-foundation/ for
// design decisions.
package webdav

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// webdavMethods lists every HTTP method the WebDAV handlers accept.
// OPTIONS is first so it can be registered separately (bypasses auth).
var webdavMethods = []string{
	http.MethodOptions,
	"PROPFIND",
	"PROPPATCH",
	http.MethodGet,
	http.MethodHead,
	http.MethodPut,
	http.MethodDelete,
	"MKCOL",
	"COPY",
	"MOVE",
}

// davAllowHeader is the value of the Allow: header in OPTIONS responses.
// It lists every method registered in webdavMethods.
const davAllowHeader = "OPTIONS, PROPFIND, PROPPATCH, GET, HEAD, PUT, DELETE, MKCOL, COPY, MOVE"

// Routes registers all WebDAV HTTP routes on router. OPTIONS is
// registered without the auth middleware (RFC 4918 §9.1 allows
// unauthenticated server discovery — Finder and Windows Mini-Redirector
// both probe this way). Every other method rides through
// resolveWebDAVAuth before reaching handlePath.
func Routes(router *echo.Group) {
	router.OPTIONS("/files", handleOptions)
	router.OPTIONS("/files/*", handleOptions)

	authed := router.Group("", resolveWebDAVAuth)
	nonOptionsMethods := webdavMethods[1:] // skip OPTIONS
	authed.Match(nonOptionsMethods, "/files", handlePath)
	authed.Match(nonOptionsMethods, "/files/*", handlePath)
}

// NextcloudRoutes registers the Nextcloud-compatible /remote.php/webdav/*
// routes. These serve the exact same handlers as /dav/files/* instead of
// redirecting — HTTP clients (including OnlyOffice mobile) strip the
// Authorization header on redirects, breaking auth after a 308.
//
// Called from web/routing.go with the same middleware chain as /dav.
func NextcloudRoutes(router *echo.Group) {
	router.OPTIONS("/webdav", handleOptions)
	router.OPTIONS("/webdav/*", handleOptions)

	authed := router.Group("", resolveWebDAVAuth)
	nonOptionsMethods := webdavMethods[1:]
	authed.Match(nonOptionsMethods, "/webdav", handlePath)
	authed.Match(nonOptionsMethods, "/webdav/*", handlePath)
}
