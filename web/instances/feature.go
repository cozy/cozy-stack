package instances

import (
	"encoding/json"
	"net/http"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/labstack/echo/v4"
)

func getFeatureFlags(c echo.Context) error {
	inst, err := lifecycle.GetInstance(c.Param("domain"))
	if err != nil {
		return wrapError(err)
	}
	return c.JSON(http.StatusOK, inst.FeatureFlags)
}

func patchFeatureFlags(c echo.Context) error {
	inst, err := lifecycle.GetInstance(c.Param("domain"))
	if err != nil {
		return wrapError(err)
	}
	var patch map[string]interface{}
	if err := json.NewDecoder(c.Request().Body).Decode(&patch); err != nil {
		return wrapError(err)
	}
	if inst.FeatureFlags == nil {
		inst.FeatureFlags = make(map[string]interface{})
	}
	for k, v := range patch {
		if v == nil {
			delete(inst.FeatureFlags, k)
		} else {
			inst.FeatureFlags[k] = v
		}
	}
	if err := couchdb.UpdateDoc(couchdb.GlobalDB, inst); err != nil {
		return wrapError(err)
	}
	return c.JSON(http.StatusOK, inst.FeatureFlags)
}
