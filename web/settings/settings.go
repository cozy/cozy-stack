// Package settings regroups some API methods to facilitate the usage of the
// io.cozy settings documents.
package settings

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/session"
	csettings "github.com/cozy/cozy-stack/model/settings"
	"github.com/cozy/cozy-stack/model/token"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

type apiSession struct {
	s *session.Session
}

func (s *apiSession) ID() string                             { return s.s.ID() }
func (s *apiSession) Rev() string                            { return s.s.Rev() }
func (s *apiSession) DocType() string                        { return consts.Sessions }
func (s *apiSession) Clone() couchdb.Doc                     { return s }
func (s *apiSession) SetID(_ string)                         {}
func (s *apiSession) SetRev(_ string)                        {}
func (s *apiSession) Relationships() jsonapi.RelationshipMap { return nil }
func (s *apiSession) Included() []jsonapi.Object             { return nil }
func (s *apiSession) Links() *jsonapi.LinksList              { return nil }
func (s *apiSession) MarshalJSON() ([]byte, error)           { return json.Marshal(s.s) }

// HTTPHandler handle all the `/settings` routes.
type HTTPHandler struct {
	svc csettings.Service
}

// NewHTTPHandler instantiates a new [HTTPHandler].
func NewHTTPHandler(svc csettings.Service) *HTTPHandler {
	return &HTTPHandler{svc}
}

func (h *HTTPHandler) getSessions(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	if err := middlewares.AllowWholeType(c, permission.GET, consts.Sessions); err != nil {
		return err
	}

	sessions, err := session.GetAll(inst)
	if err != nil {
		return err
	}

	objs := make([]jsonapi.Object, len(sessions))
	for i, s := range sessions {
		objs[i] = &apiSession{s}
	}

	return jsonapi.DataList(c, http.StatusOK, objs, nil)
}

func (h *HTTPHandler) listWarnings(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	// Any request with a token can ask for the context (no permissions are required)
	if _, err := middlewares.GetPermission(c); err != nil && !isMovedError(err) {
		return err
	}

	w := middlewares.ListWarnings(inst)

	if len(w) == 0 {
		// Sends a 404 when there is no warnings
		resp := c.Response()
		resp.Header().Set(echo.HeaderContentType, jsonapi.ContentType)
		resp.WriteHeader(http.StatusNotFound)
		_, err := resp.Write([]byte("{\"errors\": []}"))
		return err
	}

	return jsonapi.DataErrorList(c, w...)
}

// postEmail handle POST /settings/email
func (h *HTTPHandler) postEmail(c echo.Context) error {
	type body struct {
		Passphrase string `json:"passphrase"`
		Email      string `json:"email"`
	}

	if err := middlewares.AllowWholeType(c, permission.POST, consts.Settings); err != nil {
		return err
	}

	var args body
	err := c.Bind(&args)
	if err != nil {
		return jsonapi.BadJSON()
	}

	inst := middlewares.GetInstance(c)

	err = h.svc.StartEmailUpdate(inst, &csettings.UpdateEmailCmd{
		Passphrase: []byte(args.Passphrase),
		Email:      args.Email,
	})

	switch {
	case err == nil:
		c.NoContent(http.StatusNoContent)
		return nil
	case errors.Is(err, instance.ErrInvalidPassphrase):
		return jsonapi.BadRequest(instance.ErrInvalidPassphrase)
	default:
		return jsonapi.InternalServerError(err)
	}
}

// postEmailResend handle POST /settings/email/resend
func (h *HTTPHandler) postEmailResend(c echo.Context) error {
	if err := middlewares.AllowWholeType(c, permission.POST, consts.Settings); err != nil {
		return err
	}

	inst := middlewares.GetInstance(c)

	err := h.svc.ResendEmailUpdate(inst)

	switch {
	case err == nil:
		c.NoContent(http.StatusNoContent)
		return nil
	case errors.Is(err, instance.ErrInvalidPassphrase):
		return jsonapi.BadRequest(instance.ErrInvalidPassphrase)
	default:
		return jsonapi.InternalServerError(err)
	}
}

// deleteEmail handle DELETE /settings/email
func (h *HTTPHandler) deleteEmail(c echo.Context) error {
	if err := middlewares.AllowWholeType(c, permission.POST, consts.Settings); err != nil {
		return err
	}

	inst := middlewares.GetInstance(c)

	err := h.svc.CancelEmailUpdate(inst)
	switch {
	case err == nil:
		c.NoContent(http.StatusNoContent)
		return nil
	case errors.Is(err, csettings.ErrNoPendingEmail):
		return jsonapi.BadRequest(csettings.ErrNoPendingEmail)
	default:
		return jsonapi.InternalServerError(err)
	}
}

func (h *HTTPHandler) getEmailConfirmation(c echo.Context) error {
	tok := c.QueryParam("token")
	inst := middlewares.GetInstance(c)

	settingsURL := inst.SubDomain("settings").String()

	err := h.svc.ConfirmEmailUpdate(inst, tok)
	switch {
	case err == nil:
		// Redirect to the setting page
		return c.Redirect(http.StatusTemporaryRedirect, settingsURL)
	case errors.Is(err, csettings.ErrNoPendingEmail), errors.Is(err, token.ErrInvalidToken):
		return c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Domain":       inst.ContextualDomain(),
			"ContextName":  inst.ContextName,
			"Locale":       inst.Locale,
			"Title":        inst.TemplateTitle(),
			"Favicon":      middlewares.Favicon(inst),
			"Illustration": "/images/generic-error.svg",
			"ErrorTitle":   "Error InvalidToken Title",
			"Error":        "Error InvalidToken Message",
			"Link":         "Error InvalidToken Link",
			"LinkURL":      settingsURL,
			"SupportEmail": inst.SupportEmailAddress(),
		})
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, err)
	}
}

func isMovedError(err error) bool {
	j, ok := err.(*jsonapi.Error)
	return ok && j.Code == "moved"
}

// Register all the `/settings` routes to the given router.
func (h *HTTPHandler) Register(router *echo.Group) {
	router.GET("/disk-usage", h.diskUsage)

	router.POST("/email", h.postEmail)
	router.POST("/email/resend", h.postEmailResend)
	router.DELETE("/email", h.deleteEmail)
	router.GET("/email/confirm", h.getEmailConfirmation)

	router.GET("/passphrase", h.getPassphraseParameters)
	router.POST("/passphrase", h.registerPassphrase)
	router.POST("/passphrase/flagship", h.registerPassphraseFlagship)
	router.PUT("/passphrase", h.updatePassphrase)
	router.POST("/passphrase/check", h.checkPassphrase)
	router.GET("/hint", h.getHint)
	router.PUT("/hint", h.updateHint)

	router.GET("/capabilities", h.getCapabilities)
	router.GET("/instance", h.getInstance)
	router.PUT("/instance", h.updateInstance)
	router.POST("/instance/deletion", h.askInstanceDeletion)
	router.PUT("/instance/auth_mode", h.updateInstanceAuthMode)
	router.PUT("/instance/sign_tos", h.updateInstanceTOS)
	router.DELETE("/instance/moved_from", h.clearMovedFrom)

	router.GET("/flags", h.getFlags)

	router.GET("/sessions", h.getSessions)

	router.GET("/clients", h.listClients)
	router.DELETE("/clients/:id", h.revokeClient)
	router.GET("/clients/limit-exceeded", h.limitExceeded)
	router.POST("/synchronized", h.synchronized)

	router.GET("/onboarded", h.onboarded)
	router.GET("/context", h.context)
	router.GET("/warnings", h.listWarnings)
}
