package instances

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"github.com/cozy/cozy-stack/model/feature"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/config/config"
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

func getContextFromConfig(context string) (interface{}, error) {
	contexts := config.GetConfig().Contexts

	if context != "" {
		ctx, ok := contexts[context]
		if ok {
			return ctx, nil
		}
	}

	ctx, ok := contexts[config.DefaultInstanceContext]
	if ok && ctx != nil {
		return ctx, nil
	}

	return nil, fmt.Errorf("Unable to get context %q from config", context)
}

func getFeatureConfig(c echo.Context) error {
	context, err := getContextFromConfig(c.Param("context"))
	if err != nil {
		return wrapError(err)
	}
	ctx := context.(map[string]interface{})

	normalized := make(map[string]interface{})
	if m, ok := ctx["features"].(map[interface{}]interface{}); ok {
		for k, v := range m {
			normalized[fmt.Sprintf("%v", k)] = v
		}
	} else if items, ok := ctx["features"].([]interface{}); ok {
		for _, item := range items {
			if m, ok := item.(map[interface{}]interface{}); ok && len(m) == 1 {
				for k, v := range m {
					normalized[fmt.Sprintf("%v", k)] = v
				}
			} else {
				normalized[fmt.Sprintf("%v", item)] = true
			}
		}
	}

	return c.JSON(http.StatusOK, normalized)
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
