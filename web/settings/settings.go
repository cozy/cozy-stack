// Package settings regroups some API methods to facilitate the usage of the
// io.cozy settings documents. For example, it has a route for getting a CSS
// with some CSS variables that can be used as a theme.
package settings

import (
	"bytes"
	"encoding/hex"
	"html/template"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/settings"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

var themeTemplate = template.Must(template.New("theme").Parse(`:root {
	--logo-url: url({{.Logo}});
	--base00-color: {{.Base00}};
	--base01-color: {{.Base01}};
	--base02-color: {{.Base02}};
	--base03-color: {{.Base03}};
	--base04-color: {{.Base04}};
	--base05-color: {{.Base05}};
	--base06-color: {{.Base06}};
	--base07-color: {{.Base07}};
	--base08-color: {{.Base08}};
	--base09-color: {{.Base09}};
	--base0A-color: {{.Base0A}};
	--base0B-color: {{.Base0B}};
	--base0C-color: {{.Base0C}};
	--base0D-color: {{.Base0D}};
	--base0E-color: {{.Base0E}};
	--base0F-color: {{.Base0F}};
}`))

// ThemeCSS responds with a CSS that declared some variables
func ThemeCSS(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	theme, err := settings.DefaultTheme(instance)
	if err != nil {
		if couchdb.IsNotFoundError(err) {
			return echo.NewHTTPError(http.StatusNotFound, err)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err)
	}
	buffer := new(bytes.Buffer)
	err = themeTemplate.Execute(buffer, theme)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err)
	}
	return c.Blob(http.StatusOK, "text/css", buffer.Bytes())
}

func registerPassphrase(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	registerToken, err := hex.DecodeString(c.FormValue("registerToken"))
	if err != nil {
		return jsonapi.NewError(http.StatusBadRequest, err)
	}

	passphrase := []byte(c.FormValue("passphrase"))
	if err := instance.RegisterPassphrase(passphrase, registerToken); err != nil {
		return jsonapi.BadRequest(err)
	}

	if _, err := auth.SetCookieForNewSession(c); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func updatePassphrase(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	newPassphrase := []byte(c.FormValue("new-passphrase"))
	currentPassphrase := []byte(c.FormValue("current-passphrase"))
	if err := instance.UpdatePassphrase(newPassphrase, currentPassphrase); err != nil {
		return jsonapi.BadRequest(err)
	}

	if _, err := auth.SetCookieForNewSession(c); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// Routes sets the routing for the settings service
func Routes(router *echo.Group) {
	router.GET("/theme.css", ThemeCSS)

	router.POST("/passphrase", registerPassphrase)
	router.PUT("/passphrase", updatePassphrase)
}
