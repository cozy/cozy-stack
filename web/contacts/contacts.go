// Package contacts exposes a route for the myself document.
package contacts

import (
	"encoding/json"
	"net/http"

	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

type apiMyself struct{ *contact.Contact }

func (m *apiMyself) MarshalJSON() ([]byte, error)           { return json.Marshal(m.Contact) }
func (m *apiMyself) Links() *jsonapi.LinksList              { return &jsonapi.LinksList{Self: "/contacts/myself"} }
func (m *apiMyself) Relationships() jsonapi.RelationshipMap { return jsonapi.RelationshipMap{} }
func (m *apiMyself) Included() []jsonapi.Object             { return []jsonapi.Object{} }

// MyselfHandler is the handler for POST /contacts/myself. It returns the
// information about the io.cozy.contacts document for the owner of this
// instance, the "myself" contact. If the document does not exist, it is
// recreated with some basic fields.
func MyselfHandler(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	myself, err := contact.GetMyself(inst)
	if err == contact.ErrNotFound {
		var settings *couchdb.JSONDoc
		settings, err = inst.SettingsDocument()
		if err == nil {
			myself, err = contact.CreateMyself(inst, settings)
		}
	}
	if err != nil {
		return err
	}

	if err := middlewares.Allow(c, permission.GET, myself); err != nil {
		return err
	}

	return jsonapi.Data(c, http.StatusOK, &apiMyself{myself}, nil)
}

// Routes sets the routing for the contacts.
func Routes(router *echo.Group) {
	router.POST("/myself", MyselfHandler)
}
