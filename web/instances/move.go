package instances

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/pkg/globals"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/move"
	workers "github.com/cozy/cozy-stack/pkg/workers/mails"
	"github.com/labstack/echo"
)

func exporter(c echo.Context) error {
	domain := c.Param("domain")
	instance, err := instance.Get(domain)
	if err != nil {
		return err
	}
	filename, err := move.Export(instance)
	if err != nil {
		return err
	}

	link := fmt.Sprintf("http://%s%s%s", domain, c.Path(), filename)
	msg, err := jobs.NewMessage(workers.Options{
		Mode:           workers.ModeNoReply,
		TemplateName:   "archiver",
		TemplateValues: map[string]string{"ArchiveLink": link},
	})
	if err != nil {
		return err
	}

	broker := globals.GetBroker()
	_, err = broker.PushJob(&jobs.JobRequest{
		Domain:     instance.Domain,
		WorkerType: "sendmail",
		Message:    msg,
	})
	if err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

func importer(c echo.Context) error {
	domain := c.Param("domain")
	instance, err := instance.Get(domain)
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
