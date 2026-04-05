package webdav

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// handleOptions responds to OPTIONS requests. Per RFC 4918 §10.1 we
// advertise DAV class 1 (no LOCK) and the Allow header listing the
// methods currently supported. Phase 1 exposes read-only methods only.
//
// This handler MUST NOT call the VFS — it is unauthenticated and any
// side effect would be a capability leak. It also emits MS-Author-Via
// to please the Windows Mini-Redirector, which refuses to upgrade a
// connection to WebDAV without it.
func handleOptions(c echo.Context) error {
	h := c.Response().Header()
	h.Set("DAV", "1")
	h.Set("Allow", davAllowHeader)
	h.Set("MS-Author-Via", "DAV")
	return c.NoContent(http.StatusOK)
}

// handlePath is the dispatcher for every non-OPTIONS WebDAV request.
// It will grow to cover PROPFIND (plan 01-07), GET/HEAD (plan 01-08),
// and later Phase 2/3 methods. Until those plans land, every method
// returns 501 Not Implemented through the canonical sendWebDAVError
// XML body so clients get a consistent error shape.
func handlePath(c echo.Context) error {
	switch c.Request().Method {
	case "PROPFIND":
		// Implemented in plan 01-07
		return sendWebDAVError(c, http.StatusNotImplemented, "not-implemented")
	case http.MethodGet, http.MethodHead:
		// Implemented in plan 01-08
		return sendWebDAVError(c, http.StatusNotImplemented, "not-implemented")
	default:
		// Phase 2/3 methods — not yet implemented
		return sendWebDAVError(c, http.StatusNotImplemented, "not-implemented")
	}
}
