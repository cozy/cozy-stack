// Package webdav implements an RFC 4918 Class 1 WebDAV server exposing
// the Cozy instance /files/ tree. See .planning/phases/01-foundation/ for
// design decisions.
package webdav

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// webdavMethods lists every HTTP method the WebDAV handlers accept.
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

// davAllowHeader is the value of the Allow: header in OPTIONS responses.
// It lists every method registered in webdavMethods.
const davAllowHeader = "OPTIONS, PROPFIND, GET, HEAD, PUT, DELETE, MKCOL, COPY, MOVE"

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

// NextcloudRedirect replies 308 Permanent Redirect from
// /remote.php/webdav/* to the equivalent /dav/files/* path. The 308
// status code is critical: 301/302 allow clients to downgrade to GET,
// which would break PROPFIND; 308 preserves the request method.
//
// Exported so it can be wired from web/routing.go at the Echo root
// (outside the /dav group).
func NextcloudRedirect(c echo.Context) error {
	original := c.Request().URL.Path
	newPath := strings.Replace(original, "/remote.php/webdav", "/dav/files", 1)
	if raw := c.Request().URL.RawQuery; raw != "" {
		newPath += "?" + raw
	}
	return c.Redirect(http.StatusPermanentRedirect, newPath)
}
