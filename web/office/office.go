package office

import (
	"net/http"
	"os"
	"strconv"

	"github.com/cozy/cozy-stack/model/office"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// Open returns the parameters to open an office document.
func Open(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	fileID := c.Param("id")
	open, err := office.Open(inst, fileID)
	if err != nil {
		return wrapError(err)
	}
	pdoc, err := middlewares.GetPermission(c)
	if err != nil {
		return err
	}

	// If a directory is shared by link and contains a note, the note can be
	// opened with the same sharecode as the directory. The sharecode is also
	// used to identify the member that previews a sharing.
	if pdoc.Type == permission.TypeShareByLink || pdoc.Type == permission.TypeSharePreview {
		code := middlewares.GetRequestToken(c)
		open.AddShareByLinkCode(code)
	}

	sharingID := c.QueryParam("SharingID") // Cozy to Cozy sharing
	if err := open.CheckPermission(pdoc, sharingID); err != nil {
		return middlewares.ErrForbidden
	}

	memberIndex, _ := strconv.Atoi(c.QueryParam("MemberIndex"))
	readOnly := c.QueryParam("ReadOnly") == "true"
	doc, err := open.GetResult(memberIndex, readOnly)
	if err != nil {
		return wrapError(err)
	}

	return jsonapi.Data(c, http.StatusOK, doc, nil)
}

// Callback is the handler for OnlyOffice callback requests.
// Cf https://api.onlyoffice.com/editors/callback
func Callback(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	var params office.CallbackParameters
	if err := c.Bind(&params); err != nil {
		inst.Logger().WithField("nspace", "office").
			Warnf("Cannot bind callback parameters: %s", err)
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "Invalid request"})
	}

	if err := office.Callback(inst, params); err != nil {
		inst.Logger().WithField("nspace", "office").
			Infof("Error on the callback: %s", err)
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, echo.Map{"error": 0})
}

// Routes sets the routing for the collaborative edition of office documents.
func Routes(router *echo.Group) {
	router.GET("/:id/open", Open)
	router.POST("/callback", Callback)
}

func wrapError(err error) *jsonapi.Error {
	switch err {
	case office.ErrNoServer, office.ErrInvalidFile, sharing.ErrCannotOpenFile:
		return jsonapi.NotFound(err)
	case office.ErrInternalServerError:
		return jsonapi.InternalServerError(err)
	case os.ErrNotExist, vfs.ErrParentDoesNotExist, vfs.ErrParentInTrash:
		return jsonapi.NotFound(err)
	case sharing.ErrMemberNotFound:
		return jsonapi.NotFound(err)
	}
	return jsonapi.InternalServerError(err)
}
