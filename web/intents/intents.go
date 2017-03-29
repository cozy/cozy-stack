package intents

import (
	"encoding/json"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/intents"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

type apiIntent struct {
	doc *intents.Intent
}

func (i *apiIntent) ID() string                             { return i.doc.ID() }
func (i *apiIntent) Rev() string                            { return i.doc.Rev() }
func (i *apiIntent) DocType() string                        { return consts.Intents }
func (i *apiIntent) SetID(id string)                        { i.doc.SetID(id) }
func (i *apiIntent) SetRev(rev string)                      { i.doc.SetRev(rev) }
func (i *apiIntent) Relationships() jsonapi.RelationshipMap { return nil }
func (i *apiIntent) Included() []jsonapi.Object             { return nil }
func (i *apiIntent) Links() *jsonapi.LinksList {
	// TODO permissions link
	return &jsonapi.LinksList{Self: "/intents/" + i.ID()}
}
func (i *apiIntent) MarshalJSON() ([]byte, error) {
	// TODO overwrite the client in JSON-API response
	return json.Marshal(i.doc)
}

func createIntent(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	intent := &intents.Intent{}
	if _, err := jsonapi.Bind(c.Request(), intent); err != nil {
		return err
	}
	intent.SetID("")
	intent.SetRev("")
	intent.Services = nil
	intent.Client = "..." // TODO extract the client ID from the app token
	if err := intent.Save(instance); err != nil {
		return err // TODO wrap err
	}
	if err := intent.FillServices(instance); err != nil {
		return err // TODO wrap err
	}
	if err := intent.Save(instance); err != nil {
		return err // TODO wrap err
	}
	return jsonapi.Data(c, http.StatusOK, &apiIntent{intent}, nil)
}

// Routes sets the routing for the intents service
func Routes(router *echo.Group) {
	router.POST("", createIntent)
}
