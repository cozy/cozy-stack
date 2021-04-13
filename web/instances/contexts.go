package instances

import (
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/labstack/echo/v4"
)

type contextAPI struct {
	Config           interface{}    `json:"config"`
	Context          string         `json:"context"`
	Registries       []string       `json:"registries"`
	Office           *config.Office `json:"office,omitempty"`
	ClouderyEndpoint string         `json:"cloudery_endpoint,omitempty"`
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
	configuration := config.GetConfig()
	clouderies := configuration.Clouderies
	registries := configuration.Registries
	officeConfig := configuration.Office

	// Clouderies
	var clouderyEndpoint string
	var cloudery interface{}
	cloudery, ok := clouderies[contextName]
	if !ok {
		cloudery = clouderies[config.DefaultInstanceContext]
	}
	if cloudery != nil {
		api := cloudery.(map[string]interface{})["api"]
		clouderyEndpoint = api.(map[string]interface{})["url"].(string)
	}

	// Office
	var office *config.Office
	if o, ok := officeConfig[contextName]; ok {
		office = &o
	} else if o, ok := officeConfig[config.DefaultInstanceContext]; ok {
		office = &o
	}

	// Registries
	var registriesList []string
	var registryURLs []*url.URL

	// registriesURLs contains context-specific urls and default ones
	if registryURLs, ok = registries[contextName]; !ok {
		registryURLs = registries[config.DefaultInstanceContext]
	}
	for _, url := range registryURLs {
		registriesList = append(registriesList, url.String())
	}

	return contextAPI{
		Config:           config.Normalize(cfg),
		Context:          contextName,
		Registries:       registriesList,
		Office:           office,
		ClouderyEndpoint: clouderyEndpoint,
	}
}
