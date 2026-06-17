// Package excalidraw is about the routes used to open excalidraw documents.
package excalidraw

import (
	"net/http"
	"os"
	"strconv"

	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// OpenExcalidrawURL is the API handler for GET /excalidraw/:file-id/open. It
// returns the parameters to build the URL where the excalidraw document can be
// opened.
func OpenExcalidrawURL(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	fileID := c.Param("file-id")
	open, err := sharing.OpenExcalidraw(inst, fileID)
	if err != nil {
		return wrapError(err)
	}

	pdoc, err := middlewares.GetPermission(c)
	if err != nil {
		return err
	}
	memberIndex, _ := strconv.Atoi(c.QueryParam("MemberIndex"))
	readOnly := c.QueryParam("ReadOnly") == "true"

	// If a directory is shared by link and contains an excalidraw document, the
	// document can be opened with the same sharecode as the directory. The
	// sharecode is also used to identify the member that previews a sharing.
	if pdoc.Type == permission.TypeShareByLink ||
		pdoc.Type == permission.TypeSharePreview ||
		pdoc.Type == permission.TypeShareInteract {
		code := middlewares.GetRequestToken(c)
		open.AddShareByLinkCode(code)
	}

	sharingID := c.QueryParam("SharingID") // Cozy to Cozy sharing
	if err := open.CheckPermission(pdoc, sharingID); err != nil {
		return middlewares.ErrForbidden
	}

	doc, err := open.GetResult(memberIndex, readOnly)
	if err != nil {
		return wrapError(err)
	}

	return jsonapi.Data(c, http.StatusOK, doc, nil)
}

// Routes sets the routing for opening excalidraw documents.
func Routes(router *echo.Group) {
	router.GET("/:file-id/open", OpenExcalidrawURL)
}

func wrapError(err error) *jsonapi.Error {
	switch err {
	case sharing.ErrCannotOpenFile, sharing.ErrMemberNotFound:
		return jsonapi.NotFound(err)
	case os.ErrNotExist, vfs.ErrParentDoesNotExist, vfs.ErrParentInTrash:
		return jsonapi.NotFound(err)
	}
	return jsonapi.InternalServerError(err)
}
