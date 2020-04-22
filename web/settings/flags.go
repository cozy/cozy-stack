package settings

import (
	"net/http"

	"github.com/cozy/cozy-stack/model/feature"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

type apiFlags struct {
	*feature.Flags
	include bool
}

func (f *apiFlags) Relationships() jsonapi.RelationshipMap {
	return nil
}

func (f *apiFlags) Included() []jsonapi.Object {
	if !f.include {
		return nil
	}
	included := make([]jsonapi.Object, len(f.Sources))
	for i, source := range f.Sources {
		included[i] = &apiFlags{source, false}
	}
	return included
}

func (f *apiFlags) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/settings/flags"}
}

func getFlags(c echo.Context) error {
	// Any request with a token can ask for the context (no permissions are required)
	if _, err := middlewares.GetPermission(c); err != nil {
		return echo.NewHTTPError(http.StatusForbidden)
	}

	inst := middlewares.GetInstance(c)
	flags, err := feature.GetFlags(inst)
	if err != nil {
		return err
	}
	include := c.QueryParam("include") != ""
	return jsonapi.Data(c, http.StatusOK, &apiFlags{flags, include}, nil)
}
