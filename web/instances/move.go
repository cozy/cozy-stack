package instances

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/pkg/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/move"
	workers "github.com/cozy/cozy-stack/pkg/workers/mails"
	"github.com/cozy/echo"
)

func exporter(c echo.Context) error {
	domain := c.Param("domain")
	instance, err := lifecycle.GetInstance(domain)
	if err != nil {
		return err
	}
	filename, err := move.Export(instance)
	if err != nil {
		return err
	}

	link := fmt.Sprintf("http://%s%s%s", domain, c.Path(), filename)
	msg, err := jobs.NewMessage(workers.Options{
		Mode:         workers.ModeNoReply,
		TemplateName: "archiver",
		TemplateValues: map[string]interface{}{
			"ArchiveLink": link,
		},
	})
	if err != nil {
		return err
	}

	broker := jobs.System()
	_, err = broker.PushJob(instance, &jobs.JobRequest{
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

	increaseQuota, _ := strconv.ParseBool(c.QueryParam("increase_quota"))

	err = move.Import(instance, filename, dst, increaseQuota)
	if err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
