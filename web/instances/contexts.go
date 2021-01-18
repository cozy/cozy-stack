package instances

import (
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/labstack/echo/v4"
)

type contextAPI struct {
	Config           interface{} `json:"config"`
	Context          string      `json:"context"`
	Registries       []string    `json:"registries"`
	ClouderyEndpoint string      `json:"cloudery_endpoint"`
}

func showContext(c echo.Context) error {
	contextName := c.Param("name")
	contexts := config.GetConfig().Contexts
	cfg, ok := contexts[contextName].(map[string]interface{})
	if !ok {
		return c.NoContent(http.StatusNotFound)
	}
	return c.JSON(http.StatusOK, getContextAPI(contextName, cfg))
}

func lsContexts(c echo.Context) error {
	contexts := config.GetConfig().Contexts

	result := []contextAPI{}
	for contextName, ctx := range contexts {
		cfg, ok := ctx.(map[string]interface{})
		if !ok {
			cfg = map[string]interface{}{}
		}
		result = append(result, getContextAPI(contextName, cfg))
	}
	return c.JSON(http.StatusOK, result)
}

func getContextAPI(contextName string, cfg map[string]interface{}) contextAPI {
	clouderies := config.GetConfig().Clouderies
	registries := config.GetConfig().Registries

	// Clouderies
	var clouderyEndpoint string
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
	var registriesList []string
	var registryURLs []*url.URL

	// registriesURLs contains context-specific urls and default ones
	if registryURLs, ok = registries[contextName]; !ok {
		registryURLs = registries["default"]
	}
	for _, url := range registryURLs {
		registriesList = append(registriesList, url.String())
	}

	return contextAPI{
		Config:           config.Normalize(cfg),
		Context:          contextName,
		Registries:       registriesList,
		ClouderyEndpoint: clouderyEndpoint,
	}
}
