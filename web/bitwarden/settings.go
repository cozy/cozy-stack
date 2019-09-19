package bitwarden

import (
	"net/http"

	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// https://github.com/bitwarden/jslib/blob/master/src/models/response/domainsResponse.ts
type domainsResponse struct {
	EquivalentDomains       [][]string `json:"EquivalentDomains"`
	GlobalEquivalentDomains []int      `json:"GlobalEquivalentDomains"`
	Object                  string     `json:"Object"`
}

func newDomainsResponse(settings *settings.Settings) *domainsResponse {
	return &domainsResponse{
		EquivalentDomains:       settings.EquivalentDomains,
		GlobalEquivalentDomains: settings.GlobalEquivalentDomains,
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
	settings, err := settings.Get(inst)
	if err != nil {
		return err
	}

	domains := newDomainsResponse(settings)
	return c.JSON(http.StatusOK, domains)
}
