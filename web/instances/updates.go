package instances

import (
	"strconv"

	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/pkg/workers/updates"
	"github.com/cozy/echo"
)

func updatesHandler(c echo.Context) error {
	slugs := utils.SplitTrimString(c.QueryParam("Slugs"), ",")
	domain := c.QueryParam("Domain")
	forceRegistry, _ := strconv.ParseBool(c.QueryParam("ForceRegistry"))
	msg, err := jobs.NewMessage(&updates.Options{
		Slugs:         slugs,
		Force:         true,
		ForceRegistry: forceRegistry,
		Domain:        domain,
		AllDomains:    domain == "",
	})
	if err != nil {
		return err
	}
	_, err = jobs.System().PushJob(prefixer.GlobalPrefixer, &jobs.JobRequest{
		WorkerType: "updates",
		Message:    msg,
	})
	return wrapError(err)
}
