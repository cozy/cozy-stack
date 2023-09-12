package settings

import (
	"fmt"
	"net/http"

	"github.com/cozy/cozy-stack/model/feature"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

type apiClientsUsage struct {
	Limit         *int `json:"limit,omitempty"`
	Count         int  `json:"count"`
	LimitReached  bool `json:"limitReached"`
	LimitExceeded bool `json:"limitExceeded"`
}

func (j *apiClientsUsage) ID() string                             { return consts.ClientsUsageID }
func (j *apiClientsUsage) Rev() string                            { return "" }
func (j *apiClientsUsage) DocType() string                        { return consts.Settings }
func (j *apiClientsUsage) Clone() couchdb.Doc                     { return j }
func (j *apiClientsUsage) SetID(_ string)                         {}
func (j *apiClientsUsage) SetRev(_ string)                        {}
func (j *apiClientsUsage) Relationships() jsonapi.RelationshipMap { return nil }
func (j *apiClientsUsage) Included() []jsonapi.Object             { return nil }
func (j *apiClientsUsage) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/settings/clients-usage"}
}

// Settings objects permissions are only on ID
func (j *apiClientsUsage) Fetch(field string) []string { return nil }

func (h *HTTPHandler) clientsUsage(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	var result apiClientsUsage

	if err := middlewares.Allow(c, permission.GET, &result); err != nil {
		return err
	}

	flags, err := feature.GetFlags(inst)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Errorf("Could not get flags: %w", err))
	}

	limit := -1
	if clientsLimit, ok := flags.M["cozy.oauthclients.max"].(float64); ok && clientsLimit >= 0 {
		limit = int(clientsLimit)
	}

	clients, _, err := oauth.GetConnectedUserClients(inst, 100, "")
	if err != nil {
		return fmt.Errorf("Could not get user OAuth clients: %w", err)
	}
	count := len(clients)

	if limit != -1 {
		result.Limit = &limit

		if count >= limit {
			result.LimitReached = true
		}
		if count > limit {
			result.LimitExceeded = true
		}
	}
	result.Count = count
	return jsonapi.Data(c, http.StatusOK, &result, nil)
}
