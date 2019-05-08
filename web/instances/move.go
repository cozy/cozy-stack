package instances

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/move"
	"github.com/cozy/cozy-stack/pkg/mail"
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
	msg, err := job.NewMessage(mail.Options{
		Mode:         mail.ModeNoReply,
		TemplateName: "archiver",
		TemplateValues: map[string]interface{}{
			"ArchiveLink": link,
		},
	})
	if err != nil {
		return err
	}

	broker := job.System()
	_, err = broker.PushJob(instance, &job.JobRequest{
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
