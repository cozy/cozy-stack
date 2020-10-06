package instances

import (
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/move"
	"github.com/cozy/cozy-stack/worker/moves"
	"github.com/labstack/echo/v4"
)

func exporter(c echo.Context) error {
	domain := c.Param("domain")
	inst, err := lifecycle.GetInstance(domain)
	if err != nil {
		return err
	}

	exportOptions := moves.ExportOptions{
		ContextualDomain: domain,
	}
	msg, err := job.NewMessage(exportOptions)
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
	instance, err := lifecycle.GetInstance(domain)
	if err != nil {
		return err
	}

	dst := c.QueryParam("destination")
	if !strings.HasPrefix(dst, "/") {
		dst = "/" + dst
	}

	filename := c.QueryParam("filename")
	if filename == "" {
		filename = "cozy.tar.gz"
	}

	err = move.Import(instance, filename, dst)
	if err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
