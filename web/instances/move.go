package instances

import (
	"net/http"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/move"
	"github.com/labstack/echo/v4"
)

func exporter(c echo.Context) error {
	domain := c.Param("domain")
	inst, err := lifecycle.GetInstance(domain)
	if err != nil {
		return err
	}

	options := move.ExportOptions{
		ContextualDomain: domain,
	}
	msg, err := job.NewMessage(options)
	if err != nil {
		return err
	}

	_, err = job.System().PushJob(inst, &job.JobRequest{
		WorkerType: "export",
		Message:    msg,
	})
	if err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

func importer(c echo.Context) error {
	domain := c.Param("domain")
	inst, err := lifecycle.GetInstance(domain)
	if err != nil {
		return err
	}

	options := move.ImportOptions{
		ManifestURL: c.QueryParam("manifest_url"),
	}
	msg, err := job.NewMessage(options)
	if err != nil {
		return err
	}

	_, err = job.System().PushJob(inst, &job.JobRequest{
		WorkerType: "import",
		Message:    msg,
	})
	if err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}
