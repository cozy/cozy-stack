package middlewares

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/echo"
)

// BasicAuth use HTTP basic authentication to authenticate a user. The secret
// of the user should be stored in a file with the specified name, stored in
// one of the the config.Paths directories.
//
// The format of the secret is the same as our hashed passwords in database: a
// scrypt hash with a salt contained in the value.
func BasicAuth(secretFileName string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
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

			return next(c)
		}
	}
}
