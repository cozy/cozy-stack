package notifications

import (
	"encoding/json"
	"net/http"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/notification"
	"github.com/cozy/cozy-stack/model/notification/center"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
)

type apiNotif struct {
	n *notification.Notification
}

func (n *apiNotif) ID() string                             { return n.n.ID() }
func (n *apiNotif) Rev() string                            { return n.n.Rev() }
func (n *apiNotif) DocType() string                        { return consts.Notifications }
func (n *apiNotif) Clone() couchdb.Doc                     { return n }
func (n *apiNotif) SetID(_ string)                         {}
func (n *apiNotif) SetRev(_ string)                        {}
func (n *apiNotif) Relationships() jsonapi.RelationshipMap { return nil }
func (n *apiNotif) Included() []jsonapi.Object             { return nil }
func (n *apiNotif) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/notifications/" + n.n.ID()}
}
func (n *apiNotif) MarshalJSON() ([]byte, error) {
	return json.Marshal(n.n)
}

func createHandler(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	n := &notification.Notification{}
	if _, err := jsonapi.Bind(c.Request().Body, &n); err != nil {
		return err
	}
	perm, err := middlewares.GetPermission(c)
	if err != nil {
		return err
	}
	if err := center.Push(inst, perm, n); err != nil {
		return wrapErrors(err)
	}
	return jsonapi.Data(c, http.StatusCreated, &apiNotif{n}, nil)
}

func wrapErrors(err error) error {
	if err == nil {
		return nil
	}
	switch err {
	case center.ErrBadNotification:
		return jsonapi.BadRequest(err)
	case center.ErrUnauthorized:
		return jsonapi.Forbidden(err)
	case app.ErrNotFound:
		return jsonapi.NotFound(err)
	}
	return err
}

// Routes sets the routing for the notification service.
func Routes(router *echo.Group) {
	router.POST("", createHandler)
}
