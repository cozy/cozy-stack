package middlewares

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/labstack/echo/v4"
)

// BasicAuth use HTTP basic authentication to authenticate a user. The secret
// of the user should be stored in a file with the specified name, stored in
// one of the the config.Paths directories.
//
// The format of the secret is the same as our hashed passwords in database: a
// scrypt hash with a salt contained in the value.
func BasicAuth(secretFileName string) echo.MiddlewareFunc {
	check := func(next echo.HandlerFunc, c echo.Context) error {
		if c.QueryParam("Trace") == "true" {
			t := time.Now()
			defer func() {
				elapsed := time.Since(t)
				logger.
					WithDomain("admin").
					WithField("nspace", "trace").
					Printf("Check basic auth: %v", elapsed)
			}()
		}

		_, passphrase, ok := c.Request().BasicAuth()
		if !ok {
			return echo.NewHTTPError(http.StatusUnauthorized, "missing basic auth")
		}

		shadowFile, err := config.FindConfigFile(secretFileName)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err)
		}

		f, err := os.Open(shadowFile)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err)
		}
		defer f.Close()

		b, err := ioutil.ReadAll(f)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err)
		}
		b = bytes.TrimSpace(b)

		needUpdate, err := crypto.CompareHashAndPassphrase(b, []byte(passphrase))
		if err != nil {
			return echo.NewHTTPError(http.StatusForbidden, "bad passphrase")
		}
		if needUpdate {
			logger.
				WithDomain("admin").
				Warnf("Passphrase hash from %q needs update and should be regenerated", secretFileName)
		}

		return nil
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if err := check(next, c); err != nil {
				return err
			}

			return next(c)
		}
	}
}
