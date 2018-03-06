// Package settings regroups some API methods to facilitate the usage of the
// io.cozy settings documents.
package settings

import (
	"encoding/json"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	webpermissions "github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
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

	if err = webpermissions.Allow(c, permissions.GET, doc); err != nil {
		return err
	}

	return jsonapi.Data(c, http.StatusOK, &apiInstance{doc}, nil)
}

func updateInstance(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	doc := &couchdb.JSONDoc{}
	obj, err := jsonapi.Bind(c.Request(), doc)
	if err != nil {
		return err
	}

	doc.Type = consts.Settings
	doc.SetID(consts.InstanceSettingsID)
	doc.SetRev(obj.Meta.Rev)

	if err = webpermissions.Allow(c, webpermissions.PUT, doc); err != nil {
		return err
	}

	oldDoc, err := inst.SettingsDocument()
	if err != nil {
		return err
	}

	needUpdate := false

	if locale, ok := doc.M["locale"].(string); ok {
		delete(doc.M, "locale")
		if inst.Locale != locale {
			inst.Locale = locale
			needUpdate = true
		}
	}

	if autoUpdate, ok := doc.M["auto_update"].(bool); ok {
		delete(doc.M, "auto_upate")
		if inst.NoAutoUpdate != !autoUpdate {
			inst.NoAutoUpdate = !autoUpdate
			needUpdate = true
		}
	}

	if _, ok := doc.M["auth_mode"]; ok {
		delete(doc.M, "auth_mode")
	}

	// Only allow to change the TOS version and UUID via CLI
	pdoc, err := webpermissions.GetPermission(c)
	if err != nil || pdoc.Type != permissions.TypeCLI {
		doc.M["tos"] = oldDoc.M["tos"]
		doc.M["uuid"] = oldDoc.M["uuid"]
	}

	if needUpdate {
		if err := instance.Update(inst); err != nil {
			return err
		}
	}

	if err := couchdb.UpdateDoc(inst, doc); err != nil {
		return err
	}

	doc.M["locale"] = inst.Locale
	doc.M["onboarding_finished"] = inst.OnboardingFinished
	doc.M["auto_update"] = !inst.NoAutoUpdate
	doc.M["auth_mode"] = instance.AuthModeToString(inst.AuthMode)
	return jsonapi.Data(c, http.StatusOK, &apiInstance{doc}, nil)
}

func updateInstanceAuthMode(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	if err := webpermissions.AllowWholeType(c, permissions.PUT, consts.Settings); err != nil {
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
			if err = inst.SendMailConfirmationCode(); err != nil {
				return err
			}
			return c.NoContent(http.StatusNoContent)
		}
		if ok := inst.ValidateMailConfirmationCode(args.TwoFactorActivationCode); !ok {
			return c.NoContent(http.StatusUnprocessableEntity)
		}
	}

	inst.AuthMode = authMode
	if err = instance.Update(inst); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}
