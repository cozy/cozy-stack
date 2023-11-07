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
	Office           *contextOffice `json:"office,omitempty"`
	ClouderyEndpoint string         `json:"cloudery_endpoint,omitempty"`
	OIDC             interface{}    `json:"oidc,omitempty"`
}

type contextOffice struct {
	OnlyOfficeURL string
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
	for contextName := range config.GetConfig().Registries {
		if _, ok := contexts[contextName]; !ok {
			result = append(result, getContextAPI(contextName, nil))
		}
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
	var cloudery config.ClouderyConfig
	cloudery, ok := clouderies[contextName]
	if !ok {
		cloudery = clouderies[config.DefaultInstanceContext]
	}

	clouderyEndpoint = cloudery.API.URL

	// Office
	var office *contextOffice
	if o, ok := officeConfig[contextName]; ok {
		office = &contextOffice{OnlyOfficeURL: o.OnlyOfficeURL}
	} else if o, ok := officeConfig[config.DefaultInstanceContext]; ok {
		office = &contextOffice{OnlyOfficeURL: o.OnlyOfficeURL}
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

	// OIDC
	var oidc map[string]interface{}
	if full, ok := config.GetOIDC(contextName); ok {
		oidc = make(map[string]interface{})
		for k, v := range full {
			if k != "client_secret" {
				oidc[k] = v
			}
		}
	}

	return contextAPI{
		Config:           cfg,
		Context:          contextName,
		Registries:       registriesList,
		Office:           office,
		ClouderyEndpoint: clouderyEndpoint,
		OIDC:             oidc,
	}
}
