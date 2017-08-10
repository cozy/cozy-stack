package imexport

import (
	"encoding/base32"
	"fmt"
	"io"
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

func export(c echo.Context) error {
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

	var objet string
	lien := fmt.Sprintf("http://%s%s%s-%s", domain, c.Path(), domain, id)

	if instance.Locale == "en" {
		objet = "The archive with all your Cozy data is ready"
	} else if instance.Locale == "fr" {
		objet = "L'archive contenant toutes les données de Cozy est prête"
	}

	msg, err := jobs.NewMessage("json", workers.Options{
		Mode:         workers.ModeNoReply,
		Subject:      objet,
		TemplateName: "archiver_" + instance.Locale,
		TemplateValues: map[string]string{
			"RecipientName": domain,
			"Lien":          lien,
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

	return c.JSON(http.StatusOK, echo.Map{
		"message": "bienvenue sur la super page",
	})
}
func exportDir(c echo.Context) error {
	domID := c.Param("domain-id")
	fmt.Println(domID)

	src, err := os.Open(fmt.Sprintf("%s.tar.gz", domID))
	if err != nil {
		return err
	}
	dst, err := os.Create("cozy.tar.gz")
	if err != nil {
		return err
	}

	_, err = io.Copy(dst, src)
	if err != nil {
		return err
	}

	err = os.Remove(fmt.Sprintf("%s.tar.gz", domID))
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, echo.Map{
		"message": "bienvenue sur la super page bis",
	})
}

func importer(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	fs := instance.VFS()

	r, err := os.Open("cozy.tar.gz")
	if err != nil {
		return err
	}

	rep := c.Param("destination")
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

	err = imexport.Untardir(r, dst.ID(), instance)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, echo.Map{
		"message": "bienvenue sur la super page cozy",
	})
}

// Routes sets the routing for export
func Routes(router *echo.Group) {
	router.GET("/export/", export)
	router.HEAD("/export/", export)

	router.GET("/import/:destination", importer)
	router.HEAD("/import/", importer)

	router.GET("/export/:domain-id", exportDir)

}
