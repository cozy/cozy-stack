// Package settings regroups some API methods to facilitate the usage of the
// io.cozy settings documents. For example, it has a route for getting a CSS
// with some CSS variables that can be used as a theme.
package settings

import (
	"bytes"
	"html/template"
	"net/http"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/settings"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
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
func ThemeCSS(c *gin.Context) {
	instance := middlewares.GetInstance(c)
	theme, err := settings.DefaultTheme(instance)
	if err != nil {
		if couchdb.IsNotFoundError(err) {
			c.AbortWithError(http.StatusNotFound, err)
		} else {
			c.AbortWithError(http.StatusInternalServerError, err)
		}
		return
	}
	buffer := new(bytes.Buffer)
	err = themeTemplate.Execute(buffer, theme)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	c.Data(http.StatusOK, "text/css", buffer.Bytes())
}

// Routes sets the routing for the settings service
func Routes(router *gin.RouterGroup) {
	router.GET("/theme.css", ThemeCSS)
}
