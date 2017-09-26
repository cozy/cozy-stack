package instances

import (
	"encoding/base32"
	"fmt"
	"net/http"
	"os"

	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/imexport"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/vfs"
	workers "github.com/cozy/cozy-stack/pkg/workers/mails"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

func exporter(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	domain := instance.Domain

	tab := crypto.GenerateRandomBytes(20)
	id := base32.StdEncoding.EncodeToString(tab)

	w, err := os.Create(fmt.Sprintf("%s-%s.tar.gz", domain, id))
	if err != nil {
		return err
	}
	defer w.Close()

	err = imexport.Tardir(w, instance)
	if err != nil {
		return err
	}

	link := fmt.Sprintf("http://%s%s%s-%s", domain, c.Path(), domain, id)
	subject := "The archive with all your Cozy data is ready"
	if instance.Locale == "fr" {
		subject = "L'archive contenant toutes les données de Cozy est prête"
	}
	msg, err := jobs.NewMessage("json", workers.Options{
		Mode:         workers.ModeNoReply,
		Subject:      subject,
		TemplateName: "archiver_" + instance.Locale,
		TemplateValues: map[string]string{
			"RecipientName": domain,
			"Link":          link,
		},
	})
	if err != nil {
		return err
	}

	context := jobs.NewWorkerContext(instance.Domain, "abcd")
	err = workers.SendMail(context, msg)
	if err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent, nil)
}

func importer(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	fs := instance.VFS()

	filename := c.QueryParam("filename")
	if filename == "" {
		filename = "cozy.tar.gz"
	}
	r, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer r.Close()

	rep := c.QueryParam("destination")
	rep = fmt.Sprintf("/%s", rep)

	exist, err := vfs.DirExists(fs, rep)
	if err != nil {
		return err
	}
	var dst *vfs.DirDoc
	if !exist {
		dst, err = vfs.Mkdir(fs, rep, nil)
		if err != nil {
			return err
		}
	} else {
		dst, err = fs.DirByPath(rep)
		if err != nil {
			return err
		}
	}

	err = imexport.Untardir(r, dst, instance)
	if err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent, nil)
}
