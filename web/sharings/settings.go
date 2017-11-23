package sharings

import (
	"errors"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/sharings"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

func setDestination(c echo.Context) error {
	pdoc, err := perm.GetPermission(c)
	if err != nil || pdoc.Type != permissions.TypeWebapp {
		return jsonapi.BadRequest(errors.New("Invalid request"))
	}
	slug, err := extractSlugFromSourceID(pdoc.SourceID)
	if err != nil {
		return err
	}

	doctype := c.Param("doctype")
	if doctype == "" {
		return jsonapi.BadRequest(errors.New("Missing doctype"))
	}
	if doctype != consts.Files {
		return jsonapi.BadRequest(errors.New("Not supported doctype"))
	}

	dirID := c.QueryParam(consts.QueryParamDirID)
	if dirID == "" {
		return jsonapi.BadRequest(errors.New("Missing directory id"))
	}
	ins := middlewares.GetInstance(c)
	if _, err = ins.VFS().DirByID(dirID); err != nil {
		return jsonapi.BadRequest(errors.New("Directory does not exist"))
	}

	err = sharings.UpdateApplicationDestinationDirID(ins, slug, doctype, dirID)
	if err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}
