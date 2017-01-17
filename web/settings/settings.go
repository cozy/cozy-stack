// Package settings regroups some API methods to facilitate the usage of the
// io.cozy settings documents. For example, it has a route for getting a CSS
// with some CSS variables that can be used as a theme.
package settings

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/settings"
	"github.com/cozy/cozy-stack/pkg/vfs"
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

type apiDiskUsage struct {
	Used int64 `json:"used,string"`
}

func (j *apiDiskUsage) ID() string                             { return consts.DiskUsageID }
func (j *apiDiskUsage) Rev() string                            { return "" }
func (j *apiDiskUsage) DocType() string                        { return consts.Settings }
func (j *apiDiskUsage) SetID(_ string)                         {}
func (j *apiDiskUsage) SetRev(_ string)                        {}
func (j *apiDiskUsage) Relationships() jsonapi.RelationshipMap { return nil }
func (j *apiDiskUsage) Included() []jsonapi.Object             { return nil }
func (j *apiDiskUsage) SelfLink() string                       { return "/settings/disk-usage" }

func diskUsage(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	used, err := vfs.DiskUsage(instance)
	if err != nil {
		return err
	}
	return jsonapi.Data(c, http.StatusOK, &apiDiskUsage{used}, nil)
}

func registerPassphrase(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	args := &struct {
		Register   string `json:"register_token"`
		Passphrase string `json:"passphrase"`
	}{}
	if err := c.Bind(&args); err != nil {
		return err
	}

	registerToken, err := hex.DecodeString(args.Register)
	if err != nil {
		return jsonapi.NewError(http.StatusBadRequest, err)
	}

	passphrase := []byte(args.Passphrase)
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

	args := &struct {
		Current    string `json:"current_passphrase"`
		Passphrase string `json:"new_passphrase"`
	}{}
	if err := c.Bind(&args); err != nil {
		return err
	}

	newPassphrase := []byte(args.Passphrase)
	currentPassphrase := []byte(args.Current)
	if err := instance.UpdatePassphrase(newPassphrase, currentPassphrase); err != nil {
		return jsonapi.BadRequest(err)
	}

	if _, err := auth.SetCookieForNewSession(c); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

type apiInstance struct {
	doc *couchdb.JSONDoc
}

func (i *apiInstance) ID() string                             { return i.doc.ID() }
func (i *apiInstance) Rev() string                            { return i.doc.Rev() }
func (i *apiInstance) DocType() string                        { return consts.Settings }
func (i *apiInstance) SetID(id string)                        { i.doc.SetID(id) }
func (i *apiInstance) SetRev(rev string)                      { i.doc.SetRev(rev) }
func (i *apiInstance) Relationships() jsonapi.RelationshipMap { return nil }
func (i *apiInstance) Included() []jsonapi.Object             { return nil }
func (i *apiInstance) SelfLink() string                       { return "/settings/instance" }
func (i *apiInstance) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.doc)
}

func getInstance(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	doc := &couchdb.JSONDoc{}
	err := couchdb.GetDoc(instance, consts.Settings, consts.InstanceSettingsID, doc)
	if err != nil {
		return err
	}
	doc.M["locale"] = instance.Locale

	return jsonapi.Data(c, http.StatusOK, &apiInstance{doc}, nil)
}

func updateInstance(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	doc := &couchdb.JSONDoc{}
	obj, err := jsonapi.Bind(c.Request(), doc)
	if err != nil {
		return err
	}
	if locale, ok := doc.M["locale"].(string); ok {
		delete(doc.M, "locale")
		instance.Locale = locale
		if err := couchdb.UpdateDoc(couchdb.GlobalDB, instance); err != nil {
			return err
		}
	}
	doc.Type = consts.Settings
	doc.SetID(consts.InstanceSettingsID)
	doc.SetRev(obj.Meta.Rev)
	if err := couchdb.UpdateDoc(instance, doc); err != nil {
		return err
	}

	doc.M["locale"] = instance.Locale
	return jsonapi.Data(c, http.StatusOK, &apiInstance{doc}, nil)
}

// Routes sets the routing for the settings service
func Routes(router *echo.Group) {
	router.GET("/theme.css", ThemeCSS)
	router.GET("/disk-usage", diskUsage)

	router.POST("/passphrase", registerPassphrase)
	router.PUT("/passphrase", updatePassphrase)

	router.GET("/instance", getInstance)
	router.PUT("/instance", updateInstance)
}
