package instances

import (
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/labstack/echo"
)

func updatesHandler(c echo.Context) error {
	slug := c.Param("slug")
	if slug != "" {
		return instance.UpdateAll(slug)
	}
	return instance.UpdateAll()
}
