package webdav

import (
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
)

// mountAuthOnly registers the resolveWebDAVAuth middleware and a trivial
// 200 "ok" handler for every path. Used to exercise the middleware in
// isolation, independent of the real handlers (which land in later plans).
func mountAuthOnly(g *echo.Group) {
	g.Use(resolveWebDAVAuth)
	ok := func(c echo.Context) error { return c.String(http.StatusOK, "ok") }
	g.Any("/files", ok)
	g.Any("/files/*", ok)
}

func TestAuth_MissingAuthorization_Returns401WithBasicRealm(t *testing.T) {
	env := newWebdavTestEnv(t, mountAuthOnly)

	env.E.GET("/dav/files/").
		Expect().
		Status(http.StatusUnauthorized).
		Header("WWW-Authenticate").IsEqual(`Basic realm="Cozy"`)
}

func TestAuth_BearerToken_Success(t *testing.T) {
	env := newWebdavTestEnv(t, mountAuthOnly)

	env.E.GET("/dav/files/").
		WithHeader("Authorization", "Bearer "+env.Token).
		Expect().
		Status(http.StatusOK).
		Body().IsEqual("ok")
}

func TestAuth_BasicAuthTokenAsPassword_Success(t *testing.T) {
	env := newWebdavTestEnv(t, mountAuthOnly)

	// Cozy convention: the OAuth token travels in the Basic Auth password
	// field. The username is ignored (empty here).
	creds := base64.StdEncoding.EncodeToString([]byte(":" + env.Token))
	env.E.GET("/dav/files/").
		WithHeader("Authorization", "Basic "+creds).
		Expect().
		Status(http.StatusOK).
		Body().IsEqual("ok")
}

func TestAuth_InvalidToken_Returns401(t *testing.T) {
	env := newWebdavTestEnv(t, mountAuthOnly)

	env.E.GET("/dav/files/").
		WithHeader("Authorization", "Bearer not-a-real-token").
		Expect().
		Status(http.StatusUnauthorized).
		Header("WWW-Authenticate").IsEqual(`Basic realm="Cozy"`)
}

func TestAuth_OptionsBypassesAuth(t *testing.T) {
	env := newWebdavTestEnv(t, mountAuthOnly)

	// OPTIONS must reach the dummy handler even without any Authorization
	// header — RFC 4918 discovery and CORS preflight must not be gated on
	// credentials.
	env.E.OPTIONS("/dav/files/").
		Expect().
		Status(http.StatusOK).
		Body().IsEqual("ok")
}
