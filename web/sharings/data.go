package sharings

import (
	"errors"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/sharings"
	"github.com/cozy/cozy-stack/web/data"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

func doctypeExists(ins *instance.Instance, doctype string) bool {
	_, err := couchdb.DBStatus(ins, doctype)
	return err == nil
}

// receiveDocument stores a shared document in the Cozy.
//
// If the document to store is a "io.cozy.files" our custom handler will be
// called, otherwise we will redirect to /data.
func receiveDocument(c echo.Context) error {
	ins := middlewares.GetInstance(c)
	sharingID := c.QueryParam(consts.QueryParamSharingID)
	if sharingID == "" {
		return jsonapi.BadRequest(errors.New("Missing sharing id"))
	}

	sharing, errf := sharings.FindSharing(ins, sharingID)
	if errf != nil {
		return errf
	}

	var err error
	switch c.Param("doctype") {
	case consts.Files:
		err = creationWithIDHandler(c, ins, sharing.AppSlug)
	default:
		doctype := c.Param("doctype")
		if !doctypeExists(ins, doctype) {
			err = couchdb.CreateDB(ins, doctype)
			if err != nil {
				return err
			}
		}
		err = data.UpdateDoc(c)
	}

	if err != nil {
		return err
	}

	ins.Logger().Debugf("[sharings] Received %s: %s", c.Param("doctype"),
		c.Param("docid"))
	return c.JSON(http.StatusOK, nil)
}

// Depending on the doctype this function does two things:
// 1. If it's a file, its content is updated.
// 2. If it's a JSON document, its content is updated and a check is performed
//    to see if the document is still shared after the update. If not then it is
//    deleted.
func updateDocument(c echo.Context) error {
	ins := middlewares.GetInstance(c)
	ins.Logger().Debugf("[sharings] Updating %s: %s", c.Param("doctype"),
		c.Param("docid"))

	var err error
	switch c.Param("doctype") {
	case consts.Files:
		err = updateFile(c)
	default:
		err = data.UpdateDoc(c)
		if err != nil {
			return err
		}

		// TODO uncomment this code when RemoveDocumentIfNotShared will be fixed
		// ins := middlewares.GetInstance(c)
		// err = sharings.RemoveDocumentIfNotShared(ins, c.Param("doctype"), c.Param("docid"))
	}

	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, nil)
}

func deleteDocument(c echo.Context) error {
	ins := middlewares.GetInstance(c)
	ins.Logger().Debugf("[sharings] Deleting %s: %s", c.Param("doctype"),
		c.Param("docid"))

	var err error
	switch c.Param("doctype") {
	case consts.Files:
		err = trashHandler(c)

	default:
		err = data.DeleteDoc(c)
	}

	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, nil)
}
