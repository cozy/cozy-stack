package settings

import (
	"github.com/cozy/cozy-stack/model/settings"
	"github.com/labstack/echo/v4"
)

var handler *HTTPHandler

func Init(svc settings.Service) {
	handler = NewHTTPHandler(svc)
}

func Routes(router *echo.Group) {
	if handler == nil {
		panic("settings.handler not set")
	}

	handler.Register(router)
}
