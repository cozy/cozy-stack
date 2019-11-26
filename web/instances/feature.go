package instances

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"github.com/cozy/cozy-stack/model/feature"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/consts"
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

func getFeatureSets(c echo.Context) error {
	inst, err := lifecycle.GetInstance(c.Param("domain"))
	if err != nil {
		return wrapError(err)
	}
	return c.JSON(http.StatusOK, inst.FeatureSets)
}

func putFeatureSets(c echo.Context) error {
	inst, err := lifecycle.GetInstance(c.Param("domain"))
	if err != nil {
		return wrapError(err)
	}
	var list []string
	if err := json.NewDecoder(c.Request().Body).Decode(&list); err != nil {
		return wrapError(err)
	}
	sort.Strings(list)
	inst.FeatureSets = list
	if err := couchdb.UpdateDoc(couchdb.GlobalDB, inst); err != nil {
		return wrapError(err)
	}
	return c.JSON(http.StatusOK, inst.FeatureSets)
}

func getFeatureContext(c echo.Context) error {
	id := fmt.Sprintf("%s.%s", consts.ContextFlagsSettingsID, c.Param("context"))
	var flags feature.Flags
	err := couchdb.GetDoc(couchdb.GlobalDB, consts.Settings, id, &flags)
	if err != nil && !couchdb.IsNotFoundError(err) {
		return wrapError(err)
	}
	return c.JSON(http.StatusOK, flags.M)
}

type contextParameters struct {
	Ratio float64     `json:"ratio"`
	Value interface{} `json:"value"`
}

func patchFeatureContext(c echo.Context) error {
	id := fmt.Sprintf("%s.%s", consts.ContextFlagsSettingsID, c.Param("context"))
	var flags couchdb.JSONDoc
	err := couchdb.GetDoc(couchdb.GlobalDB, consts.Settings, id, &flags)
	if err != nil && !couchdb.IsNotFoundError(err) {
		return wrapError(err)
	}

	var patch map[string][]contextParameters
	if err := json.NewDecoder(c.Request().Body).Decode(&patch); err != nil {
		return wrapError(err)
	}
	if flags.M == nil {
		flags.M = make(map[string]interface{})
	}
	flags.Type = consts.Settings
	flags.SetID(id)
	for k, v := range patch {
		if len(v) == 0 {
			delete(flags.M, k)
		} else {
			flags.M[k] = v
		}
	}
	if err := couchdb.Upsert(couchdb.GlobalDB, &flags); err != nil {
		return wrapError(err)
	}

	delete(flags.M, "_id")
	delete(flags.M, "_rev")
	return c.JSON(http.StatusOK, flags.M)
}

func getFeatureDefaults(c echo.Context) error {
	var defaults feature.Flags
	err := couchdb.GetDoc(couchdb.GlobalDB, consts.Settings, consts.DefaultFlagsSettingsID, &defaults)
	if err != nil && !couchdb.IsNotFoundError(err) {
		return wrapError(err)
	}
	return c.JSON(http.StatusOK, defaults.M)
}

func patchFeatureDefaults(c echo.Context) error {
	var defaults couchdb.JSONDoc
	err := couchdb.GetDoc(couchdb.GlobalDB, consts.Settings, consts.DefaultFlagsSettingsID, &defaults)
	if err != nil && !couchdb.IsNotFoundError(err) {
		return wrapError(err)
	}

	var patch map[string]interface{}
	if err := json.NewDecoder(c.Request().Body).Decode(&patch); err != nil {
		return wrapError(err)
	}
	if defaults.M == nil {
		defaults.M = make(map[string]interface{})
	}
	defaults.Type = consts.Settings
	defaults.SetID(consts.DefaultFlagsSettingsID)
	for k, v := range patch {
		if v == nil {
			delete(defaults.M, k)
		} else {
			defaults.M[k] = v
		}
	}
	if err := couchdb.Upsert(couchdb.GlobalDB, &defaults); err != nil {
		return wrapError(err)
	}

	delete(defaults.M, "_id")
	delete(defaults.M, "_rev")
	return c.JSON(http.StatusOK, defaults.M)
}
