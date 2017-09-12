package instances

import (
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/labstack/echo"
)

func updatesHandler(c echo.Context) error {
	slugs := utils.SplitTrimString(c.QueryParam("Slugs"), ",")
	domain := c.QueryParam("Domain")
	if domain != "" {
		inst, err := instance.Get(domain)
		if err != nil {
			return wrapError(err)
		}
		return wrapError(instance.UpdateInstance(inst, slugs...))
	}
	return wrapError(instance.UpdateAll(true, slugs...))
}
