package bitwarden

import (
	"encoding/json"
	"net/http"

	"github.com/cozy/cozy-stack/model/bitwarden"
	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// https://github.com/bitwarden/jslib/blob/master/common/src/models/response/globalDomainResponse.ts
type globalDomainsReponse struct {
	Type     int      `json:"Type"`
	Domains  []string `json:"Domains"`
	Excluded bool     `json:"Excluded"`
}

// https://github.com/bitwarden/jslib/blob/master/common/src/models/response/domainsResponse.ts
type domainsResponse struct {
	EquivalentDomains       [][]string             `json:"EquivalentDomains"`
	GlobalEquivalentDomains []globalDomainsReponse `json:"GlobalEquivalentDomains"`
	Object                  string                 `json:"Object"`
}

func newDomainsResponse(setting *settings.Settings) *domainsResponse {
	globals := make([]globalDomainsReponse, 0, len(bitwarden.GlobalDomains))
	for k, v := range bitwarden.GlobalDomains {
		excluded := false
		for _, domain := range setting.GlobalEquivalentDomains {
			if bitwarden.GlobalEquivalentDomainsType(domain) == k {
				excluded = true
				break
			}
		}
		globals = append(globals, globalDomainsReponse{
			Type:     int(k),
			Domains:  v,
			Excluded: excluded,
		})
	}
	return &domainsResponse{
		EquivalentDomains:       setting.EquivalentDomains,
		GlobalEquivalentDomains: globals,
		Object:                  "domains",
	}
}

// GetDomains is the handler for listing the domains in settings.
func GetDomains(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.GET, consts.BitwardenProfiles); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid token",
		})
	}
	setting, err := settings.Get(inst)
	if err != nil {
		return err
	}

	domains := newDomainsResponse(setting)
	return c.JSON(http.StatusOK, domains)
}

// UpdateDomains is the handler for updating the domains in settings.
func UpdateDomains(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.PUT, consts.BitwardenProfiles); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid token",
		})
	}

	var req struct {
		Equivalent [][]string `json:"equivalentDomains"`
		Global     []int      `json:"globalEquivalentDomains"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid JSON",
		})
	}

	setting, err := settings.Get(inst)
	if err != nil {
		return err
	}

	setting.EquivalentDomains = req.Equivalent
	setting.GlobalEquivalentDomains = req.Global
	if err := setting.Save(inst); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	domains := newDomainsResponse(setting)
	return c.JSON(http.StatusOK, domains)
}
