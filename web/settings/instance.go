// Package settings regroups some API methods to facilitate the usage of the
// io.cozy settings documents.
package settings

import (
	"encoding/json"
	"net/http"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
)

type apiInstance struct {
	doc *couchdb.JSONDoc
}

func (i *apiInstance) ID() string                             { return i.doc.ID() }
func (i *apiInstance) Rev() string                            { return i.doc.Rev() }
func (i *apiInstance) DocType() string                        { return consts.Settings }
func (i *apiInstance) Clone() couchdb.Doc                     { return i }
func (i *apiInstance) SetID(id string)                        { i.doc.SetID(id) }
func (i *apiInstance) SetRev(rev string)                      { i.doc.SetRev(rev) }
func (i *apiInstance) Relationships() jsonapi.RelationshipMap { return nil }
func (i *apiInstance) Included() []jsonapi.Object             { return nil }
func (i *apiInstance) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/settings/instance"}
}
func (i *apiInstance) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.doc)
}

func getInstance(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	doc, err := inst.SettingsDocument()
	if err != nil {
		return err
	}

	doc.M["locale"] = inst.Locale
	doc.M["onboarding_finished"] = inst.OnboardingFinished
	doc.M["auto_update"] = !inst.NoAutoUpdate
	doc.M["auth_mode"] = instance.AuthModeToString(inst.AuthMode)
	doc.M["tos"] = inst.TOSSigned
	doc.M["tos_latest"] = inst.TOSLatest
	doc.M["uuid"] = inst.UUID
	doc.M["context"] = inst.ContextName

	if err = middlewares.Allow(c, permission.GET, doc); err != nil {
		return err
	}

	return jsonapi.Data(c, http.StatusOK, &apiInstance{doc}, nil)
}

func updateInstance(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	doc := &couchdb.JSONDoc{}
	obj, err := jsonapi.Bind(c.Request().Body, doc)
	if err != nil {
		return err
	}

	doc.Type = consts.Settings
	doc.SetID(consts.InstanceSettingsID)
	doc.SetRev(obj.Meta.Rev)

	if err = middlewares.Allow(c, permission.PUT, doc); err != nil {
		return err
	}

	pdoc, err := middlewares.GetPermission(c)
	if err != nil || pdoc.Type != permission.TypeCLI {
		delete(doc.M, "auth_mode")
		delete(doc.M, "tos")
		delete(doc.M, "tos_latest")
		delete(doc.M, "uuid")
		delete(doc.M, "context")
	}

	if err := lifecycle.Patch(inst, &lifecycle.Options{SettingsObj: doc}); err != nil {
		return err
	}

	doc.M["locale"] = inst.Locale
	doc.M["onboarding_finished"] = inst.OnboardingFinished
	doc.M["auto_update"] = !inst.NoAutoUpdate
	doc.M["auth_mode"] = instance.AuthModeToString(inst.AuthMode)
	doc.M["tos"] = inst.TOSSigned
	doc.M["tos_latest"] = inst.TOSLatest
	doc.M["uuid"] = inst.UUID
	doc.M["context"] = inst.ContextName

	return jsonapi.Data(c, http.StatusOK, &apiInstance{doc}, nil)
}

func updateInstanceTOS(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	// Allow any request from OAuth tokens to use this route
	pdoc, err := middlewares.GetPermission(c)
	if err != nil {
		return err
	}
	if pdoc.Type != permission.TypeOauth && pdoc.Type != permission.TypeCLI {
		return echo.NewHTTPError(http.StatusForbidden)
	}

	if err := lifecycle.ManagerSignTOS(inst, c.Request()); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

func updateInstanceAuthMode(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	if err := middlewares.AllowWholeType(c, permission.PUT, consts.Settings); err != nil {
		return err
	}

	args := struct {
		AuthMode                string `json:"auth_mode"`
		TwoFactorActivationCode string `json:"two_factor_activation_code"`
	}{}
	if err := c.Bind(&args); err != nil {
		return err
	}

	authMode, err := instance.StringToAuthMode(args.AuthMode)
	if err != nil {
		return jsonapi.BadRequest(err)
	}
	if inst.HasAuthMode(authMode) {
		return c.NoContent(http.StatusNoContent)
	}

	switch authMode {
	case instance.Basic:
	case instance.TwoFactorMail:
		if args.TwoFactorActivationCode == "" {
			if err = lifecycle.SendMailConfirmationCode(inst); err != nil {
				return err
			}
			return c.NoContent(http.StatusNoContent)
		}
		if ok := inst.ValidateMailConfirmationCode(args.TwoFactorActivationCode); !ok {
			return c.NoContent(http.StatusUnprocessableEntity)
		}
	}

	err = lifecycle.Patch(inst, &lifecycle.Options{AuthMode: args.AuthMode})
	if err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}
