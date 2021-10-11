package oauth

import (
	"encoding/json"
	"net/http"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/labstack/echo/v4"
)

func deleteClients(c echo.Context) error {
	domain := c.Param("domain")
	inst, err := lifecycle.GetInstance(domain)
	if err != nil {
		return err
	}

	kind := c.QueryParam("Kind")
	var clients []couchdb.Doc
	err = couchdb.ForeachDocs(inst, consts.OAuthClients, func(id string, doc json.RawMessage) error {
		client := &oauth.Client{}
		if err := json.Unmarshal(doc, client); err != nil {
			return err
		}
		if kind == "" || client.ClientKind == kind {
			clients = append(clients, client)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if len(clients) > 0 {
		if err := couchdb.BulkDeleteDocs(inst, consts.OAuthClients, clients); err != nil {
			return err
		}
	}

	return c.JSON(http.StatusOK, echo.Map{"count": len(clients)})
}

// Routes sets the routing for the oauth clients (admin)
func Routes(router *echo.Group) {
	router.DELETE("/:domain/clients", deleteClients)
}
