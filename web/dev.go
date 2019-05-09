package web

import (
	"fmt"
	"net/http"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/mail"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/statik"
	"github.com/cozy/cozy-stack/worker/mails"
	"github.com/cozy/echo"
)

// devMailHandler allow to easily render a mail from a route of the stack. The
// query parameters are used as data input for the mail template. The
// ContentType query parameter allow to render the mail in "text/html" or
// "text/plain".
func devMailsHandler(c echo.Context) error {
	name := c.Param("name")
	locale := c.QueryParam("locale")
	if locale == "" {
		locale = statik.GetLanguageFromHeader(c.Request().Header)
	}

	recipientName := c.QueryParam("RecipientName")
	if recipientName == "" {
		recipientName = "Jean Dupont"
	}

	layout := c.QueryParam("layout")
	if layout == "" {
		layout = "layout"
	}

	data := devData(c)
	j := &job.Job{JobID: "1", Domain: data["Domain"].(string)}
	inst := middlewares.GetInstance(c)
	ctx := job.NewWorkerContext("0", j, inst)
	_, parts, err := mails.RenderMail(ctx, name, layout, locale, recipientName, data)
	if err != nil {
		return err
	}

	contentType := c.QueryParam("ContentType")
	if contentType == "" {
		contentType = "text/html"
	}

	var part *mail.Part
	for _, p := range parts {
		if p.Type == contentType {
			part = p
		}
	}
	if part == nil {
		return echo.NewHTTPError(http.StatusNotFound,
			fmt.Errorf("Could not find template %q with content-type %q", name, contentType))
	}

	// Remove all CSP policies to display HTML email. this is a dev-only
	// handler, no need to worry.
	c.Response().Header().Set(echo.HeaderContentSecurityPolicy, "")
	if part.Type == "text/html" {
		return c.HTML(http.StatusOK, part.Body)
	}
	return c.String(http.StatusOK, part.Body)
}

// devTemplatesHandler allow to easily render a given template from a route of
// the stack. The query parameters are used as data input for the template.
func devTemplatesHandler(c echo.Context) error {
	name := c.Param("name")
	return c.Render(http.StatusOK, name, devData(c))
}

func devData(c echo.Context) echo.Map {
	data := make(echo.Map)
	for k, v := range c.QueryParams() {
		if len(v) > 0 {
			data[k] = v[0]
		}
	}
	if _, ok := data["Domain"]; !ok {
		data["Domain"] = c.Request().Host
	}
	if _, ok := data["ContextName"]; !ok {
		data["ContextName"] = config.DefaultInstanceContext
	}
	if i, err := lifecycle.GetInstance(c.Request().Host); err == nil {
		data["CozyUI"] = middlewares.CozyUI(i)
		data["ThemeCSS"] = middlewares.ThemeCSS(i)
		data["Favicon"] = middlewares.Favicon(i)
		data["InstanceURL"] = i.PageURL("/", nil)
	}
	return data
}
