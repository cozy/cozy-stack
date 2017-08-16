package settings

import (
	"bytes"
	"html/template"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/settings"
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
