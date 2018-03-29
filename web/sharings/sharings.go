package sharings

import (
	"errors"
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/contacts"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/sharing"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	perm "github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

// CreateSharing initializes a new sharing (on the sharer)
func CreateSharing(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	var s sharing.Sharing
	obj, err := jsonapi.Bind(c.Request().Body, &s)
	if err != nil {
		return jsonapi.BadJSON()
	}

	slug, err := checkCreatePermissions(c, &s)
	if err != nil {
		return echo.NewHTTPError(http.StatusForbidden)
	}

	if err = s.BeOwner(inst, slug); err != nil {
		return wrapErrors(err)
	}

	if rel, ok := obj.GetRelationship("recipients"); ok {
		if data, ok := rel.Data.([]interface{}); ok {
			for _, ref := range data {
				if id, ok := ref.(map[string]interface{})["id"].(string); ok {
					if err = s.AddContact(inst, id); err != nil {
						return err
					}
				}
			}
		}
	}

	codes, err := s.Create(inst)
	if err != nil {
		return wrapErrors(err)
	}
	if err = s.SendMails(inst, codes); err != nil {
		return wrapErrors(err)
	}
	as := &sharing.APISharing{
		Sharing:     &s,
		Credentials: nil,
	}
	return jsonapi.Data(c, http.StatusCreated, as, nil)
}

// PutSharing creates a sharing request (on the recipient's cozy)
func PutSharing(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	var s sharing.Sharing
	obj, err := jsonapi.Bind(c.Request().Body, &s)
	if err != nil {
		return jsonapi.BadJSON()
	}
	s.SID = obj.ID

	if err := s.CreateRequest(inst); err != nil {
		return wrapErrors(err)
	}
	as := &sharing.APISharing{
		Sharing:     &s,
		Credentials: nil,
	}
	return jsonapi.Data(c, http.StatusCreated, as, nil)
}

// GetSharing returns the sharing document associated to the given sharingID.
// The requester must have the permission on at least one doctype declared in
// the sharing document.
func GetSharing(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}
	if err = checkGetPermissions(c, s); err != nil {
		return wrapErrors(err)
	}
	as := &sharing.APISharing{
		Sharing:     s,
		Credentials: nil,
	}
	return jsonapi.Data(c, http.StatusOK, as, nil)
}

// AnswerSharing is used to exchange credentials between 2 cozys, after the
// recipient has accepted a sharing.
func AnswerSharing(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}
	var creds sharing.Credentials
	if _, err = jsonapi.Bind(c.Request().Body, &creds); err != nil {
		return jsonapi.BadJSON()
	}
	ac, err := s.ProcessAnswer(inst, &creds)
	if err != nil {
		return wrapErrors(err)
	}
	return jsonapi.Data(c, http.StatusOK, ac, nil)
}

func renderDiscoveryForm(c echo.Context, inst *instance.Instance, code int, sharingID, state string, m *sharing.Member) error {
	publicName, _ := inst.PublicName()
	return c.Render(code, "sharing_discovery.html", echo.Map{
		"Domain":        inst.Domain,
		"Locale":        inst.Locale,
		"PublicName":    publicName,
		"RecipientCozy": m.Instance,
		"RecipientName": m.Name,
		"SharingID":     sharingID,
		"State":         state,
		"URLError":      code != http.StatusOK,
	})
}

// GetDiscovery displays a form where a recipient can give the adress of their
// cozy instance
func GetDiscovery(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	state := c.QueryParam("state")

	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Domain": inst.Domain,
			"Error":  "Error Invalid sharing id",
		})
	}

	member, err := s.FindMemberByState(inst, state)
	if err != nil {
		return c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Domain": inst.Domain,
			"Error":  "Error Invalid state",
		})
	}

	return renderDiscoveryForm(c, inst, http.StatusOK, sharingID, state, member)
}

// PostDiscovery is called when the recipient has given its Cozy URL. Either an
// error is returned or the recipient will be redirected to their cozy.
//
// Note: we don't have an anti-CSRF system, we rely on shareCode being secret.
func PostDiscovery(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	state := c.FormValue("state")
	sharecode := c.FormValue("sharecode")
	cozyURL := c.FormValue("url")

	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}

	var member *sharing.Member
	if sharecode != "" {
		member, err = s.FindMemberBySharecode(inst, sharecode)
		if err != nil {
			return wrapErrors(err)
		}
	} else {
		member, err = s.FindMemberByState(inst, state)
		if err != nil {
			return wrapErrors(err)
		}
	}
	if err = s.RegisterCozyURL(inst, member, cozyURL); err != nil {
		return wrapErrors(err)
	}

	redirectURL, err := member.GenerateOAuthURL(s)
	if err != nil {
		return wrapErrors(err)
	}
	if c.Request().Header.Get("Accept") == "application/json" {
		return c.JSON(http.StatusOK, echo.Map{
			"redirect": redirectURL,
		})
	}
	return c.Redirect(http.StatusFound, redirectURL)
}

// Routes sets the routing for the sharing service
func Routes(router *echo.Group) {
	// Create a sharing
	router.POST("/", CreateSharing)        // On the sharer
	router.PUT("/:sharing-id", PutSharing) // On a recipient
	router.GET("/:sharing-id", GetSharing)
	router.POST("/:sharing-id/answer", AnswerSharing)

	// Register the URL of their Cozy for recipients
	router.GET("/:sharing-id/discovery", GetDiscovery)
	router.POST("/:sharing-id/discovery", PostDiscovery)

	// Replicator routes
	replicatorRoutes(router)
}

func extractSlugFromSourceID(sourceID string) (string, error) {
	parts := strings.SplitN(sourceID, "/", 2)
	if len(parts) < 2 {
		return "", jsonapi.BadRequest(errors.New("Invalid request"))
	}
	slug := parts[1]
	return slug, nil
}

// checkCreatePermissions checks the sharer's token has all the permissions
// matching the ones defined in the sharing document
func checkCreatePermissions(c echo.Context, s *sharing.Sharing) (string, error) {
	requestPerm, err := perm.GetPermission(c)
	if err != nil {
		return "", err
	}
	if requestPerm.Type != permissions.TypeWebapp &&
		requestPerm.Type != permissions.TypeOauth {
		return "", permissions.ErrInvalidAudience
	}
	// TODO add tests
	for _, r := range s.Rules {
		pr := permissions.Rule{
			Title:    r.Title,
			Type:     r.DocType,
			Verbs:    permissions.ALL,
			Selector: r.Selector,
			Values:   r.Values,
		}
		if !requestPerm.Permissions.RuleInSubset(pr) {
			return "", echo.NewHTTPError(http.StatusForbidden)
		}
	}
	if requestPerm.Type == permissions.TypeOauth {
		return "", nil
	}
	return extractSlugFromSourceID(requestPerm.SourceID)
}

// checkGetPermissions checks the requester's token has at least one doctype
// permission declared in the rules of the sharing document
func checkGetPermissions(c echo.Context, s *sharing.Sharing) error {
	requestPerm, err := perm.GetPermission(c)
	if err != nil {
		return err
	}

	// TODO add tests
	if requestPerm.Type == permissions.TypeSharePreview &&
		requestPerm.SourceID == consts.Sharings+"/"+s.SID {
		return nil
	}
	if requestPerm.Type != permissions.TypeWebapp &&
		requestPerm.Type != permissions.TypeOauth {
		return permissions.ErrInvalidAudience
	}

	for _, r := range s.Rules {
		pr := permissions.Rule{
			Title:    r.Title,
			Type:     r.DocType,
			Verbs:    permissions.Verbs(permissions.GET),
			Selector: r.Selector,
			Values:   r.Values,
		}
		if requestPerm.Permissions.RuleInSubset(pr) {
			return nil
		}
	}
	return echo.NewHTTPError(http.StatusForbidden)
}

// wrapErrors returns a formatted error
func wrapErrors(err error) error {
	switch err {
	case contacts.ErrNoMailAddress:
		return jsonapi.InvalidAttribute("recipients", err)
	case sharing.ErrNoRecipients, sharing.ErrNoRules:
		return jsonapi.BadRequest(err)
	case sharing.ErrInvalidURL:
		return jsonapi.InvalidParameter("url", err)
	case sharing.ErrInvalidSharing:
		return jsonapi.BadRequest(err)
	case sharing.ErrMemberNotFound:
		return jsonapi.NotFound(err)
	case sharing.ErrMailNotSent:
		return jsonapi.BadRequest(err)
	case sharing.ErrRequestFailed:
		return jsonapi.BadGateway(err)
	case sharing.ErrNoOAuthClient:
		return jsonapi.BadRequest(err)
	case sharing.ErrMissingID, sharing.ErrMissingRev:
		return jsonapi.BadRequest(err)
	case sharing.ErrInternalServerError:
		return jsonapi.InternalServerError(err)
	}
	return err
}
