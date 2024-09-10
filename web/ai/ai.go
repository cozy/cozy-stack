package ai

import (
	"net/http"

	"github.com/cozy/cozy-stack/model/openwebui"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// Open returns the parameters to open an office document.
func Open(c echo.Context) error {
	if !middlewares.IsLoggedIn(c) && !middlewares.HasWebAppToken(c) {
		return middlewares.ErrForbidden
	}
	inst := middlewares.GetInstance(c)
	doc, err := openwebui.Open(inst)
	if err != nil {
		return jsonapi.InternalServerError(err)
	}

	return jsonapi.Data(c, http.StatusOK, doc, nil)
}

// Routes sets the routing for the AI chatbot sessions.
func Routes(router *echo.Group) {
	router.GET("/open", Open)
}
