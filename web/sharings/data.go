package sharings

import (
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/sharings"
	"github.com/cozy/cozy-stack/web/data"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

// receiveDocument stores a shared document in the Cozy.
//
// If the document to store is a "io.cozy.files" our custom handler will be
// called, otherwise we will redirect to /data.
func receiveDocument(c echo.Context) error {
	doctype := c.Param("doctype")
	docid := c.Param("docid")
	log := middlewares.GetInstance(c).Logger()
	log.Debugf("[sharings] Receiving %s: %s", doctype, docid)

	var err error
	if doctype == consts.Files {
		err = creationWithIDHandler(c)
	} else {
		err = data.UpdateDoc(c)
	}
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, nil)
}

// Depending on the doctype this function does two things:
// 1. If it's a file, its content is updated.
// 2. If it's a JSON document, its content is updated and a check is performed
//    to see if the document is still shared after the update. If not then it is
//    deleted.
func updateDocument(c echo.Context) error {
	doctype := c.Param("doctype")
	docid := c.Param("docid")
	ins := middlewares.GetInstance(c)
	ins.Logger().Debugf("[sharings] Updating %s: %s", doctype, docid)

	var err error
	if doctype == consts.Files {
		err = updateFile(c)
	} else {
		if err = data.UpdateDoc(c); err != nil {
			return err
		}
		err = sharings.RemoveDocumentIfNotShared(ins, doctype, docid)
	}
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, nil)
}

func deleteDocument(c echo.Context) error {
	doctype := c.Param("doctype")
	docid := c.Param("docid")
	log := middlewares.GetInstance(c).Logger()
	log.Debugf("[sharings] Deleting %s: %s", doctype, docid)

	var err error
	if doctype == consts.Files {
		err = trashHandler(c)
	} else {
		err = data.DeleteDoc(c)
	}
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, nil)
}
