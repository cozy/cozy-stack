package settings

import (
	"github.com/cozy/cozy-stack/model/settings"
	"github.com/cozy/cozy-stack/pkg/rabbitmq"
	"github.com/labstack/echo/v4"
)

var handler *HTTPHandler

func Init(svc settings.Service, rmq rabbitmq.Service) {
	handler = NewHTTPHandler(svc, rmq)
}

func Routes(router *echo.Group) {
	if handler == nil {
		panic("settings.handler not set")
	}

	handler.Register(router)
}
