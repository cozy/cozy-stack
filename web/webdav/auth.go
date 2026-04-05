package webdav

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// resolveWebDAVAuth is the WebDAV auth middleware. It accepts OAuth
// tokens in either Authorization: Bearer <tok> or Authorization: Basic
// base64(":<tok>") (username ignored — Cozy convention). OPTIONS requests
// bypass authentication entirely, per RFC 4918 §9.1 discovery semantics.
//
// On failure, it writes a 401 Unauthorized with
// WWW-Authenticate: Basic realm="Cozy". It does NOT emit an audit log on
// 401 — unauthenticated requests are the normal way clients discover the
// realm and would flood the logs (SEC-04 explicitly excludes them).
func resolveWebDAVAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if c.Request().Method == http.MethodOptions {
			return next(c)
		}

		tok := middlewares.GetRequestToken(c)
		if tok == "" {
			return sendWebDAV401(c)
		}
		inst := middlewares.GetInstance(c)
		pdoc, err := middlewares.ParseJWT(c, inst, tok)
		if err != nil {
			return sendWebDAV401(c)
		}
		middlewares.ForcePermission(c, pdoc)
		return next(c)
	}
}

// sendWebDAV401 writes a 401 Unauthorized response with the Basic realm
// header WebDAV clients expect. The body is empty — RFC 4918 does not
// mandate an XML body on 401 and clients don't parse it.
func sendWebDAV401(c echo.Context) error {
	c.Response().Header().Set("WWW-Authenticate", `Basic realm="Cozy"`)
	return c.NoContent(http.StatusUnauthorized)
}

// hashToken returns a short stable fingerprint of an OAuth token for
// audit logs. The token itself MUST NEVER be logged.
func hashToken(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(h[:8])
}

// auditLog emits a WARN-level log with structured fields for WebDAV
// audit events (path traversal, Depth:infinity, out-of-scope access).
// It reads the instance from the echo context. It MUST NOT be called on
// 401 paths — SEC-04 excludes unauthenticated rejections from audit logs.
func auditLog(c echo.Context, event string, normalizedPath string) {
	inst := middlewares.GetInstance(c)
	tok := middlewares.GetRequestToken(c)
	fields := logger.Fields{
		"source_ip":       c.RealIP(),
		"user_agent":      c.Request().UserAgent(),
		"method":          c.Request().Method,
		"raw_url":         c.Request().URL.String(),
		"normalized_path": normalizedPath,
	}
	if tok != "" {
		fields["token_hash"] = hashToken(tok)
	}
	if inst != nil {
		fields["instance"] = inst.Domain
		inst.Logger().WithNamespace("webdav").WithFields(fields).Warnf("%s", event)
		return
	}
	logger.WithNamespace("webdav").WithFields(fields).Warnf("%s", event)
}
