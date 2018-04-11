package instances

import (
	"strconv"

	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/echo"
)

func updatesHandler(c echo.Context) error {
	slugs := utils.SplitTrimString(c.QueryParam("Slugs"), ",")
	domain := c.QueryParam("Domain")
	forceRegistry, _ := strconv.ParseBool(c.QueryParam("ForceRegistry"))
	opts := &instance.UpdatesOptions{
		Slugs:         slugs,
		Force:         true,
		ForceRegistry: forceRegistry,
	}
	if domain != "" {
		inst, err := instance.Get(domain)
		if err != nil {
			return wrapError(err)
		}
		return wrapError(instance.UpdateInstance(inst, opts))
	}
	return wrapError(instance.UpdateAll(opts))
}
