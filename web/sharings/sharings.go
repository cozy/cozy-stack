package sharings

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"errors"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/contacts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/sharings"
	"github.com/cozy/cozy-stack/web/data"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	perm "github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

// DiscoveryErrorKey is the key for translating the discovery error message
const DiscoveryErrorKey = "URL Discovery error"

type apiRecipient struct {
	*contacts.Contact
}

func (r *apiRecipient) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.Contact)
}

func (r *apiRecipient) Relationships() jsonapi.RelationshipMap { return nil }
func (r *apiRecipient) Included() []jsonapi.Object             { return nil }
func (r *apiRecipient) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/data/io.cozy.contacts/" + r.DocID}
}

type apiReference struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

type apiSharing struct {
	*sharings.Sharing
}

func (s *apiSharing) MarshalJSON() ([]byte, error) {
	// XXX do not put the recipients (and their OAuth infos) in the response
	// TODO do the same for the sharer
	return json.Marshal(&struct {
		Recipients []*sharings.Member `json:"recipients,omitempty"`
		*sharings.Sharing
	}{
		Recipients: nil,
		Sharing:    s.Sharing,
	})
}

func (s *apiSharing) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/sharings/" + s.SID}
}

// Relationships is part of the jsonapi.Object interface
// It is used to generate the recipients relationships
func (s *apiSharing) Relationships() jsonapi.RelationshipMap {
	l := len(s.Recipients)
	data := make([]apiReference, l)
	for i, rec := range s.Recipients {
		r := rec.RefContact
		data[i] = apiReference{ID: r.ID, Type: r.Type, Status: rec.Status}
	}
	contents := jsonapi.Relationship{Data: data}
	return jsonapi.RelationshipMap{"recipients": contents}
}

// Included is part of the jsonapi.Object interface
func (s *apiSharing) Included() []jsonapi.Object {
	// TODO add the permissions in relationships + included
	var included []jsonapi.Object
	for _, rec := range s.Recipients {
		c := rec.Contact(nil)
		if c != nil {
			included = append(included, &apiRecipient{c})
		}
	}
	return included
}

var _ jsonapi.Object = (*apiSharing)(nil)
var _ jsonapi.Object = (*apiRecipient)(nil)

// CreateSharing initializes a sharing by creating the associated document,
// registering the sharer as a new OAuth client at each recipient as well as
// sending them a mail invitation.
func CreateSharing(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	params := &sharings.CreateSharingParams{}
	if err := json.NewDecoder(c.Request().Body).Decode(params); err != nil {
		return jsonapi.BadRequest(errors.New("Invalid body"))
	}
	slug, err := checkCreatePermissions(c, params)
	if err != nil {
		return wrapErrors(sharings.ErrForbidden)
	}

	sharing, err := sharings.CreateSharing(instance, params, slug)
	if err != nil {
		return wrapErrors(err)
	}
	if err = sharings.SendMails(instance, sharing); err != nil {
		return wrapErrors(err)
	}

	return jsonapi.Data(c, http.StatusCreated, &apiSharing{sharing}, nil)
}

// GetSharingDoc returns the sharing document associated to the given sharingID.
// The requester must have the permission on at least one doctype declared in
// the sharing document.
func GetSharingDoc(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	sharingID := c.Param("sharing-id")
	sharing, err := sharings.FindSharing(instance, sharingID)
	if err != nil {
		return jsonapi.NotFound(err)
	}
	if err = checkGetPermissions(c, sharing); err != nil {
		return wrapErrors(sharings.ErrForbidden)
	}
	return jsonapi.Data(c, http.StatusOK, &apiSharing{sharing}, nil)
}

// AddSharingRecipient adds an existing recipient to an existing sharing
// TODO document this route
// TODO use a query param to pass the contact id (instead of JSON)
// TODO check that the contact exists
func AddSharingRecipient(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	// Get sharing doc
	id := c.Param("id")
	sharing := &sharings.Sharing{}
	err := couchdb.GetDoc(instance, consts.Sharings, id, sharing)
	if err != nil {
		return wrapErrors(sharings.ErrSharingDoesNotExist)
	}

	// Create recipient, register, and send mail
	ref := couchdb.DocReference{}
	if err = json.NewDecoder(c.Request().Body).Decode(&ref); err != nil {
		return err
	}
	rs := sharings.Member{
		RefContact: ref,
	}
	sharing.Recipients = append(sharing.Recipients, rs)

	if err = sharings.SendMails(instance, sharing); err != nil {
		return wrapErrors(err)
	}
	return jsonapi.Data(c, http.StatusOK, &apiSharing{sharing}, nil)

}

func renderDiscoveryForm(c echo.Context, i *instance.Instance, code int, sharingID, shareCode string, recipient *contacts.Contact) error {
	urlErr := ""
	if code != http.StatusOK {
		urlErr = i.Translate(DiscoveryErrorKey)
	}
	publicName, err := i.PublicName()
	if err != nil {
		publicName = ""
	}
	recName := ""
	if mail, err := recipient.ToMailAddress(); err == nil {
		recName = mail.Name
	}
	recCozy := recipient.PrimaryCozyURL()

	return c.Render(code, "sharing_discovery.html", echo.Map{
		"Locale":        i.Locale,
		"SharingID":     sharingID,
		"ShareCode":     shareCode,
		"RecipientName": recName,
		"RecipientCozy": recCozy,
		"PublicName":    publicName,
		"URLError":      urlErr,
	})
}

func discoveryForm(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	shareCode := c.QueryParam("sharecode")

	sharing, err := sharings.FindSharing(instance, sharingID)
	if err != nil {
		return c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Error": "Error Invalid sharing id",
		})
	}
	contact, err := sharings.FindContactByShareCode(instance, sharing, shareCode)
	if err != nil {
		return c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Error": "Error Invalid sharecode",
		})
	}

	return renderDiscoveryForm(c, instance, http.StatusOK, sharingID, shareCode, contact)
}

func discovery(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	shareCode := c.FormValue("sharecode")
	cozyURL := c.FormValue("url")

	sharing, err := sharings.FindSharing(instance, sharingID)
	if err != nil {
		return wrapErrors(err)
	}

	contact, err := sharings.FindContactByShareCode(instance, sharing, shareCode)
	if err != nil {
		return c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Error": "Error Invalid sharecode",
		})
	}
	u, err := url.Parse(cozyURL)
	if err != nil {
		return wrapErrors(err)
	}
	if u.Scheme == "" {
		u.Scheme = "https" // Set https as the default scheme
	}

	member, err := sharing.GetMemberFromContactID(instance, contact.ID())
	if err != nil {
		return wrapErrors(err)
	}

	if err = sharings.RegisterClientOnTheRecipient(instance, sharing, member, u); err != nil {
		return renderDiscoveryForm(c, instance, http.StatusBadRequest, sharingID, shareCode, contact)
	}

	oAuthRedirect, err := sharings.GenerateOAuthURL(instance, sharing, member, shareCode)
	if err != nil {
		return wrapErrors(err)
	}
	return c.Redirect(http.StatusFound, oAuthRedirect)
}

// SharingAnswer handles a sharing answer from the sharer side
// TODO document this route
// TODO can we make this request idempotent?
func SharingAnswer(c echo.Context) error {
	state := c.QueryParam("state")
	clientID := c.QueryParam("client_id")
	accessCode := c.QueryParam("access_code")
	instance := middlewares.GetInstance(c)

	res, err := sharings.SharingAccepted(instance, state, clientID, accessCode)
	if err != nil {
		return wrapErrors(err)
	}
	return c.JSON(http.StatusFound, res)
}

// Routes sets the routing for the sharing service
func Routes(router *echo.Group) {
	// API endpoints for the apps
	router.POST("/destination/:doctype", setDestination)

	router.POST("/", CreateSharing)
	router.GET("/:sharing-id", GetSharingDoc)
	router.POST("/:id/recipients", AddSharingRecipient)

	// HTML pages, to be consumed by the recipients in their browser
	router.GET("/:sharing-id/discovery", discoveryForm)
	router.POST("/:sharing-id/discovery", discovery)

	// Internal routes, to be called by a cozy-stack
	router.POST("/answer", SharingAnswer)

	group := router.Group("/doc/:doctype", data.ValidDoctype)
	group.POST("/:docid", receiveDocument)
	group.PUT("/:docid", updateDocument)
	group.PATCH("/:docid", patchDirOrFile)
	group.DELETE("/:docid", deleteDocument)

	router.DELETE("/files/:file-id/referenced_by", removeReferences)

	// Revoke a sharing
	router.DELETE("/:sharing-id", revokeSharing)
	router.DELETE("/:sharing-id/:client-id", revokeRecipient)
	router.DELETE("/:sharing-id/recipient/:contact-id", revokeContact)
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
func checkCreatePermissions(c echo.Context, params *sharings.CreateSharingParams) (string, error) {
	requestPerm, err := perm.GetPermission(c)
	if err != nil {
		return "", err
	}
	if requestPerm.Type != permissions.TypeWebapp {
		return "", permissions.ErrInvalidAudience
	}
	if !params.Permissions.IsSubSetOf(requestPerm.Permissions) {
		return "", echo.NewHTTPError(http.StatusForbidden)
	}
	return extractSlugFromSourceID(requestPerm.SourceID)
}

// checkGetPermissions checks the requester's token has at least one doctype
// permission declared in the sharing document
func checkGetPermissions(c echo.Context, sharing *sharings.Sharing) error {
	ins := middlewares.GetInstance(c)
	sharingPerms, err := sharing.Permissions(ins)
	if err != nil {
		return err
	}

	requestPerm, err := perm.GetPermission(c)
	if err != nil {
		return err
	}

	for _, rule := range sharingPerms.Permissions {
		if requestPerm.Permissions.RuleInSubset(rule) {
			return nil
		}
	}
	return sharings.ErrForbidden
}

// wrapErrors returns a formatted error
func wrapErrors(err error) error {
	switch err {
	case sharings.ErrBadSharingType:
		return jsonapi.InvalidParameter("sharing_type", err)
	case sharings.ErrRecipientDoesNotExist:
		return jsonapi.NotFound(err)
	case sharings.ErrMissingScope, sharings.ErrMissingState, sharings.ErrMissingCode,
		sharings.ErrRecipientHasNoURL, sharings.ErrRecipientHasNoEmail, sharings.ErrRecipientBadParams:
		return jsonapi.BadRequest(err)
	case sharings.ErrSharingDoesNotExist, sharings.ErrPublicNameNotDefined:
		return jsonapi.NotFound(err)
	case sharings.ErrMailCouldNotBeSent:
		return jsonapi.InternalServerError(err)
	case sharings.ErrNoOAuthClient:
		return jsonapi.BadRequest(err)
	case sharings.ErrForbidden:
		return echo.NewHTTPError(http.StatusForbidden)
	}
	return err
}
