package office

import (
	"fmt"
	"net/http"
	"os"

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
	mode := "edit"
	if err := middlewares.AllowVFS(c, permission.PUT, open.File); err != nil {
		mode = "view"
		if err := middlewares.AllowVFS(c, permission.GET, open.File); err != nil {
			return err
		}
	}
	doc, err := open.GetResult(mode)
	if err != nil {
		return wrapError(err)
	}
	return jsonapi.Data(c, http.StatusOK, doc, nil)
}

// Callback is the handler for OnlyOffice callback requests.
// Cf https://api.onlyoffice.com/editors/callback
func Callback(c echo.Context) error {
	var data map[string]interface{}
	if err := c.Bind(&data); err != nil {
		fmt.Printf("err = %v\n", err)
	} else {
		fmt.Printf("data = %#v\n", data)
	}
	return c.JSON(http.StatusOK, echo.Map{"error": 0})
}

// Routes sets the routing for the collaborative edition of office documents.
func Routes(router *echo.Group) {
	router.GET("/:id/open", Open)
	router.POST("/:id/callback", Callback)
}

func wrapError(err error) *jsonapi.Error {
	switch err {
	case office.ErrInvalidFile:
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
