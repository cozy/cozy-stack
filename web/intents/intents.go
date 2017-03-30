package intents

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/intents"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	webpermissions "github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

type apiIntent struct {
	doc *intents.Intent
	ins *instance.Instance
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
	// In the JSON-API, the client is the domain of the client-side app that
	// asked the intent (it is used for postMessage)
	parts := strings.SplitN(i.doc.Client, "/", 2)
	if len(parts) < 2 {
		return nil, echo.NewHTTPError(http.StatusForbidden)
	}
	was := i.doc.Client
	i.doc.Client = i.ins.SubDomain(parts[1]).Host
	res, err := json.Marshal(i.doc)
	i.doc.Client = was
	return res, err
}

func createIntent(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	intent := &intents.Intent{}
	if _, err := jsonapi.Bind(c.Request(), intent); err != nil {
		return err // TODO wrap err
	}
	pdoc, err := webpermissions.GetPermission(c)
	if err != nil || pdoc.Type != permissions.TypeApplication {
		return echo.NewHTTPError(http.StatusForbidden)
	}
	intent.Client = pdoc.SourceID
	intent.SetID("")
	intent.SetRev("")
	intent.Services = nil
	if err = intent.Save(instance); err != nil {
		return err // TODO wrap err
	}
	if err = intent.FillServices(instance); err != nil {
		return err // TODO wrap err
	}
	if err = intent.Save(instance); err != nil {
		return err // TODO wrap err
	}
	api := &apiIntent{intent, instance}
	return jsonapi.Data(c, http.StatusOK, api, nil)
}

// Routes sets the routing for the intents service
func Routes(router *echo.Group) {
	router.POST("", createIntent)
}
