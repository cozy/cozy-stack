package instances

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/assets"
	"github.com/cozy/cozy-stack/pkg/assets/model"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/labstack/echo/v4"
)

func rebuildRedis(c echo.Context) error {
	instances, err := instance.List()
	if err != nil {
		return wrapError(err)
	}
	if err = job.System().CleanRedis(); err != nil {
		return wrapError(err)
	}
	for _, i := range instances {
		err = job.System().RebuildRedis(i)
		if err != nil {
			return wrapError(err)
		}
	}
	return c.NoContent(http.StatusNoContent)
}

// Renders the assets list loaded in memory and served by the cozy
func assetsInfos(c echo.Context) error {
	assetsMap, err := assets.List()
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, assetsMap)
}

func addAssets(c echo.Context) error {
	var unmarshaledAssets []model.AssetOption
	if err := json.NewDecoder(c.Request().Body).Decode(&unmarshaledAssets); err != nil {
		return err
	}

	err := assets.Add(unmarshaledAssets)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": err.Error()})
	}
	return nil
}

func deleteAssets(c echo.Context) error {
	context := c.Param("context")
	name := c.Param("*")

	err := assets.Remove(name, context)
	if err != nil {
		return wrapError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func lsContexts(c echo.Context) error {
	type contextAPI struct {
		Context          string   `json:"context"`
		Registries       []string `json:"registries"`
		ClouderyEndpoint string   `json:"cloudery_endpoint"`
	}

	contexts := config.GetConfig().Contexts
	clouderies := config.GetConfig().Clouderies
	registries := config.GetConfig().Registries

	result := []contextAPI{}
	for contextName := range contexts {
		var clouderyEndpoint string
		var registriesList []string

		// Clouderies
		var cloudery interface{}

		cloudery, ok := clouderies[contextName]

		if !ok {
			cloudery = clouderies["default"]
		}

		if cloudery != nil {
			api := cloudery.(map[string]interface{})["api"]
			clouderyEndpoint = api.(map[string]interface{})["url"].(string)
		}

		// Registries
		var registryURLs []*url.URL

		// registriesURLs contains context-specific urls and default ones
		if registryURLs, ok = registries[contextName]; !ok {
			registryURLs = registries["default"]
		}
		for _, url := range registryURLs {
			registriesList = append(registriesList, url.String())
		}

		result = append(result, contextAPI{
			Context:          contextName,
			Registries:       registriesList,
			ClouderyEndpoint: clouderyEndpoint,
		})
	}
	return c.JSON(http.StatusOK, result)
}
