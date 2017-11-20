package sharings

import (
	"encoding/json"
	"net/http"
	"net/url"

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

type apiSharing struct {
	*sharings.Sharing
}

func (s *apiSharing) MarshalJSON() ([]byte, error) {
	// XXX do not put the recipients (and their OAuth infos) in the response
	return json.Marshal(&struct {
		Recipients []*sharings.RecipientStatus `json:"recipients,omitempty"`
		*sharings.Sharing
	}{
		Recipients: nil,
		Sharing:    s.Sharing,
	})
}

func (s *apiSharing) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/sharings/" + s.SID}
}

type apiReference struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

// Relationships is part of the jsonapi.Object interface
// It is used to generate the recipients relationships
func (s *apiSharing) Relationships() jsonapi.RelationshipMap {
	l := len(s.RecipientsStatus)
	data := make([]apiReference, l)
	for i, rec := range s.RecipientsStatus {
		r := rec.RefRecipient
		data[i] = apiReference{ID: r.ID, Type: r.Type, Status: rec.Status}
	}
	contents := jsonapi.Relationship{Data: data}
	return jsonapi.RelationshipMap{"recipients": contents}
}

// Included is part of the jsonapi.Object interface
func (s *apiSharing) Included() []jsonapi.Object {
	var included []jsonapi.Object
	for _, rec := range s.RecipientsStatus {
		r := rec.GetCachedRecipient()
		if r != nil {
			included = append(included, &apiRecipient{r})
		}
	}
	return included
}

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

var _ jsonapi.Object = (*apiSharing)(nil)
var _ jsonapi.Object = (*apiRecipient)(nil)

// CreateSharing initializes a sharing by creating the associated document,
// registering the sharer as a new OAuth client at each recipient as well as
// sending them a mail invitation.
func CreateSharing(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	sharing := new(sharings.Sharing)
	if err := json.NewDecoder(c.Request().Body).Decode(sharing); err != nil {
		return err
	}
	// TODO we should enforce the app_slug
	if err := checkCreatePermissions(c, sharing); err != nil {
		return err
	}
	err := sharings.CreateSharing(instance, sharing)
	if err != nil {
		return wrapErrors(err)
	}

	err = sharings.SendSharingMails(instance, sharing)
	if err != nil {
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
		return err
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
	rs := &sharings.RecipientStatus{
		RefRecipient: ref,
	}
	sharing.RecipientsStatus = append(sharing.RecipientsStatus, rs)

	if err = sharings.RegisterRecipient(instance, rs); err != nil {
		return wrapErrors(err)
	}
	if err = sharings.SendSharingMails(instance, sharing); err != nil {
		return wrapErrors(err)
	}
	return jsonapi.Data(c, http.StatusOK, &apiSharing{sharing}, nil)

}

// SharingRequest handles a sharing request from the recipient side.
// It creates a temporary sharing document and redirects to the authorize page.
// TODO: this route should be protected against 'DDoS' attacks: one could spam
// this route to force a doc creation at each request
// TODO document this route
// TODO can we use a verb that is not GET?
// TODO can we make this request idempotent?
func SharingRequest(c echo.Context) error {
	scope := c.QueryParam("scope")
	state := c.QueryParam("state")
	sharingType := c.QueryParam("sharing_type")
	desc := c.QueryParam("description")
	clientID := c.QueryParam("client_id")
	appSlug := c.QueryParam(consts.QueryParamAppSlug)

	instance := middlewares.GetInstance(c)

	sharing, err := sharings.CreateSharingRequest(instance, desc, state,
		sharingType, scope, clientID, appSlug)
	if err == sharings.ErrSharingAlreadyExist {
		redirectAuthorize := instance.PageURL("/auth/authorize", c.QueryParams())
		return c.Redirect(http.StatusSeeOther, redirectAuthorize)
	}
	if err != nil {
		return wrapErrors(err)
	}
	// Particular case for master-master: register the sharer
	if sharingType == consts.MasterMasterSharing {
		if err = sharings.RegisterSharer(instance, sharing); err != nil {
			return wrapErrors(err)
		}
		if err = sharings.SendClientID(sharing); err != nil {
			return wrapErrors(err)
		}
	} else if sharing.SharingType == consts.MasterSlaveSharing {
		// The recipient listens deletes for a master-slave sharing
		for _, rule := range sharing.Permissions {
			err = sharings.AddTrigger(instance, rule, sharing.SharingID, true)
			if err != nil {
				return err
			}
		}
	}

	redirectAuthorize := instance.PageURL("/auth/authorize", c.QueryParams())
	return c.Redirect(http.StatusSeeOther, redirectAuthorize)
}

// RecipientRefusedSharing is called when the recipient refused the sharing (on
// the recipient side).
//
// This function will delete the sharing document and inform the sharer by
// returning her the sharing id, the client id (oauth) and nothing else (more
// especially no scope and no access code).
func RecipientRefusedSharing(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	// We collect the information we need to send to the sharer: the client id,
	// the sharing id.
	sharingID := c.FormValue("state")
	if sharingID == "" {
		return wrapErrors(sharings.ErrMissingState)
	}
	clientID := c.FormValue("client_id")
	if clientID == "" {
		return wrapErrors(sharings.ErrNoOAuthClient)
	}

	redirect, err := sharings.RecipientRefusedSharing(instance, sharingID)
	if err != nil {
		return wrapErrors(err)
	}
	u, err := url.ParseRequestURI(redirect)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("state", sharingID)
	q.Set("client_id", clientID)
	u.RawQuery = q.Encode()
	u.Fragment = ""

	return c.Redirect(http.StatusFound, u.String()+"#")
}

// SharingAnswer handles a sharing answer from the sharer side
// TODO document this route
// TODO can we use a verb that is not GET?
// TODO can we make this request idempotent?
func SharingAnswer(c echo.Context) error {
	var err error
	var u string

	state := c.QueryParam("state")
	clientID := c.QueryParam("client_id")
	accessCode := c.QueryParam("access_code")

	instance := middlewares.GetInstance(c)

	// The sharing is refused if there is no access code
	if accessCode != "" {
		u, err = sharings.SharingAccepted(instance, state, clientID, accessCode)
	} else {
		u, err = sharings.SharingRefused(instance, state, clientID)
	}
	if err != nil {
		return wrapErrors(err)
	}
	return c.Redirect(http.StatusFound, u)
}

func renderDiscoveryForm(c echo.Context, instance *instance.Instance, code int, recID, recEmail, sharingID string) error {
	// Send error message if the code is not 200
	var urlErr string
	if code != http.StatusOK {
		urlErr = instance.Translate(DiscoveryErrorKey)
	}

	publicName, err := instance.PublicName()
	if err != nil {
		return wrapErrors(err)
	}

	return c.Render(code, "sharing_discovery.html", echo.Map{
		"Locale":         instance.Locale,
		"RecipientID":    recID,
		"RecipientEmail": recEmail,
		"SharingID":      sharingID,
		"PublicName":     publicName,
		"URLError":       urlErr,
	})
}

func discoveryForm(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	sharingID := c.QueryParam("sharing_id")
	recipientID := c.QueryParam("recipient_id")
	recipientEmail := c.QueryParam("recipient_email")

	// Check mandatory fields
	sharing, err := sharings.FindSharing(instance, sharingID)
	if err != nil {
		return c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Error": "Error Invalid sharing id",
		})
	}
	recipient, err := sharings.GetRecipient(instance, recipientID)
	if err != nil {
		return c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Error": "Error Invalid recipient id",
		})
	}
	// Block multiple url
	if len(recipient.Cozy) > 0 {
		recStatus, err := sharing.GetRecipientStatusFromRecipientID(instance, recipient.ID())
		if err != nil {
			return wrapErrors(err)
		}
		if err = sharings.RegisterRecipient(instance, recStatus); err != nil {
			return wrapErrors(err)
		}
		// Generate the oauth URL and redirect the recipient
		oAuthRedirect, err := sharings.GenerateOAuthQueryString(sharing, recStatus, instance.Scheme())
		if err != nil {
			return wrapErrors(err)
		}
		return c.Redirect(http.StatusFound, oAuthRedirect)
	}
	if recipientEmail == "" {
		return c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Error": "Error Invalid recipient email",
		})
	}

	return renderDiscoveryForm(c, instance, http.StatusOK, recipientID, recipientEmail, sharingID)
}

func discovery(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	wantsJSON := c.Request().Header.Get("Accept") == "application/json"

	recipientURL := c.FormValue("url")
	sharingID := c.FormValue("sharing_id")
	recipientID := c.FormValue("recipient_id")
	recipientEmail := c.FormValue("recipient_email")

	sharing, err := sharings.FindSharing(instance, sharingID)
	if err != nil {
		return wrapErrors(err)
	}

	// Save the URL in db
	recipient, err := sharings.GetRecipient(instance, recipientID)
	if err != nil {
		return wrapErrors(err)
	}
	recURL, err := url.Parse(recipientURL)
	if err != nil {
		return wrapErrors(err)
	}
	// Set https as the default scheme
	if recURL.Scheme == "" {
		recURL.Scheme = "https"
	}

	cozyURL := recURL.String()
	found := false
	for _, c := range recipient.Cozy {
		if c.URL == cozyURL {
			found = true
			break
		}
	}
	if !found {
		recipient.Cozy = append(recipient.Cozy, contacts.Cozy{URL: cozyURL})
	}

	// Register the recipient with the given URL and save in db
	recStatus, err := sharing.GetRecipientStatusFromRecipientID(instance, recipient.ID())
	if err != nil {
		return wrapErrors(err)
	}
	recStatus.ForceRecipient(recipient)
	if err = sharings.RegisterRecipient(instance, recStatus); err != nil {
		if wantsJSON {
			return c.JSON(http.StatusBadRequest, echo.Map{
				"error": instance.Translate(DiscoveryErrorKey),
			})
		}
		return renderDiscoveryForm(c, instance, http.StatusBadRequest, recipientID, recipientEmail, sharingID)
	}

	if !found {
		if err = couchdb.UpdateDoc(instance, recipient); err != nil {
			return wrapErrors(err)
		}
	}
	if err = couchdb.UpdateDoc(instance, sharing); err != nil {
		return wrapErrors(err)
	}

	// Generate the oauth URL and redirect the recipient
	oAuthRedirect, err := sharings.GenerateOAuthQueryString(sharing, recStatus, instance.Scheme())
	if err != nil {
		return wrapErrors(err)
	}
	return c.Redirect(http.StatusFound, oAuthRedirect)
}

// ReceiveClientID receives an OAuth ClientID in a master-master context.
// This is called from a recipient, after he registered himself to the sharer.
// The received clientID is called a HostClientID, as it refers to a client
// created by the sharer, i.e. the host here.
func ReceiveClientID(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	p := &sharings.SharingRequestParams{}
	if err := json.NewDecoder(c.Request().Body).Decode(p); err != nil {
		return err
	}
	sharing, rec, err := sharings.FindSharingRecipient(instance, p.SharingID, p.ClientID)
	if err != nil {
		return wrapErrors(err)
	}
	rec.HostClientID = p.HostClientID
	err = couchdb.UpdateDoc(instance, sharing)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, nil)
}

// getAccessToken asks for an Access Token, from the recipient side.
// It is called in a master-master context, after the sharer received the
// answer from the recipient.
func getAccessToken(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	p := &sharings.SharingRequestParams{}
	if err := json.NewDecoder(c.Request().Body).Decode(p); err != nil {
		return err
	}
	if p.SharingID == "" {
		return wrapErrors(sharings.ErrMissingState)
	}
	if p.Code == "" {
		return wrapErrors(sharings.ErrMissingCode)
	}
	sharing, err := sharings.FindSharing(instance, p.SharingID)
	if err != nil {
		return wrapErrors(err)
	}
	sharer := sharing.Sharer.SharerStatus
	err = sharings.ExchangeCodeForToken(instance, sharing, sharer, p.Code)
	if err != nil {
		return wrapErrors(err)
	}
	// Add triggers on the recipient side for each rule
	if sharing.SharingType == consts.MasterMasterSharing {
		for _, rule := range sharing.Permissions {
			err = sharings.AddTrigger(instance, rule, sharing.SharingID, false)
			if err != nil {
				return wrapErrors(err)
			}
		}
	}
	return c.JSON(http.StatusOK, nil)
}

// receiveDocument stores a shared document in the Cozy.
//
// If the document to store is a "io.cozy.files" our custom handler will be
// called, otherwise we will redirect to /data.
func receiveDocument(c echo.Context) error {
	ins := middlewares.GetInstance(c)
	sharingID := c.QueryParam(consts.QueryParamSharingID)
	if sharingID == "" {
		return jsonapi.BadRequest(errors.New("Missing sharing id"))
	}

	sharing, errf := sharings.FindSharing(ins, sharingID)
	if errf != nil {
		return errf
	}

	var err error
	switch c.Param("doctype") {
	case consts.Files:
		err = creationWithIDHandler(c, ins, sharing.AppSlug)
	default:
		doctype := c.Param("doctype")
		if !doctypeExists(ins, doctype) {
			err = couchdb.CreateDB(ins, doctype)
			if err != nil {
				return err
			}
		}
		err = data.UpdateDoc(c)
	}

	if err != nil {
		return err
	}

	ins.Logger().Debugf("[sharings] Received %s: %s", c.Param("doctype"),
		c.Param("docid"))
	return c.JSON(http.StatusOK, nil)
}

// Depending on the doctype this function does two things:
// 1. If it's a file, its content is updated.
// 2. If it's a JSON document, its content is updated and a check is performed
//    to see if the document is still shared after the update. If not then it is
//    deleted.
func updateDocument(c echo.Context) error {
	ins := middlewares.GetInstance(c)
	ins.Logger().Debugf("[sharings] Updating %s: %s", c.Param("doctype"),
		c.Param("docid"))

	var err error
	switch c.Param("doctype") {
	case consts.Files:
		err = updateFile(c)
	default:
		err = data.UpdateDoc(c)
		if err != nil {
			return err
		}

		ins := middlewares.GetInstance(c)
		err = sharings.RemoveDocumentIfNotShared(ins, c.Param("doctype"),
			c.Param("docid"))
	}

	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, nil)
}

func deleteDocument(c echo.Context) error {
	ins := middlewares.GetInstance(c)
	ins.Logger().Debugf("[sharings] Deleting %s: %s", c.Param("doctype"),
		c.Param("docid"))

	var err error
	switch c.Param("doctype") {
	case consts.Files:
		err = trashHandler(c)

	default:
		err = data.DeleteDoc(c)
	}

	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, nil)
}

// Set sharing to revoked and delete all associated OAuth Clients.
func revokeSharing(c echo.Context) error {
	ins := middlewares.GetInstance(c)

	sharingID := c.Param("sharing-id")
	sharing, err := sharings.FindSharing(ins, sharingID)
	if err != nil {
		return jsonapi.NotFound(err)
	}

	permType, err := checkRevokeSharingPermissions(c, sharing)
	if err != nil {
		return c.JSON(http.StatusForbidden, err)
	}
	recursive := permType == permissions.TypeWebapp

	if err = sharings.RevokeSharing(ins, sharing, recursive); err != nil {
		return wrapErrors(err)
	}
	ins.Logger().Debugf("[sharings] revokeSharing: Sharing %s was revoked", sharingID)

	return c.NoContent(http.StatusOK)
}

func revokeRecipient(c echo.Context) error {
	ins := middlewares.GetInstance(c)

	sharingID := c.Param("sharing-id")
	sharing, err := sharings.FindSharing(ins, sharingID)
	if err != nil {
		return jsonapi.NotFound(err)
	}

	recipientClientID := c.Param("client-id")

	err = checkRevokeRecipientPermissions(c, sharing, recipientClientID)
	if err != nil {
		return c.JSON(http.StatusForbidden, err)
	}

	err = sharings.RevokeRecipientByClientID(ins, sharing, recipientClientID)
	if err != nil {
		return wrapErrors(err)
	}

	return c.NoContent(http.StatusOK)
}

func revokeContact(c echo.Context) error {
	ins := middlewares.GetInstance(c)

	sharingID := c.Param("sharing-id")
	sharing, err := sharings.FindSharing(ins, sharingID)
	if err != nil {
		return jsonapi.NotFound(err)
	}

	contactID := c.Param("contact-id")

	err = checkRevokeContactPermissions(c, sharing)
	if err != nil {
		return c.JSON(http.StatusForbidden, err)
	}

	err = sharings.RevokeRecipientByContactID(ins, sharing, contactID)
	if err != nil {
		return wrapErrors(err)
	}

	return c.NoContent(http.StatusOK)
}

func setDestination(c echo.Context) error {
	pdoc, err := perm.GetPermission(c)
	if err != nil || pdoc.Type != permissions.TypeWebapp {
		return jsonapi.BadRequest(errors.New("Invalid request"))
	}
	slug := pdoc.SourceID

	doctype := c.Param("doctype")
	if doctype == "" {
		return jsonapi.BadRequest(errors.New("Missing doctype"))
	}
	if doctype != consts.Files {
		return jsonapi.BadRequest(errors.New("Not supported doctype"))
	}

	dirID := c.QueryParam(consts.QueryParamDirID)
	if dirID == "" {
		return jsonapi.BadRequest(errors.New("Missing directory id"))
	}
	ins := middlewares.GetInstance(c)
	if _, err := ins.VFS().DirByID(dirID); err != nil {
		return jsonapi.BadRequest(errors.New("Directory does not exist"))
	}

	err = sharings.UpdateApplicationDestinationDirID(ins, slug, doctype, dirID)
	if err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// Routes sets the routing for the sharing service
func Routes(router *echo.Group) {
	// API endpoints for the apps
	router.POST("/destination/:doctype", setDestination)

	router.POST("/", CreateSharing)
	router.GET("/:sharing-id", GetSharingDoc)
	router.POST("/:id/recipients", AddSharingRecipient)

	// HTML forms, to be consumed directly by a browser
	router.GET("/request", SharingRequest)
	router.POST("/formRefuse", RecipientRefusedSharing)
	router.GET("/answer", SharingAnswer)

	router.GET("/discovery", discoveryForm)
	router.POST("/discovery", discovery)

	// Internal routes, to be called by a cozy-stack
	router.POST("/access/client", ReceiveClientID)
	router.POST("/access/code", getAccessToken)

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

func checkRevokeContactPermissions(c echo.Context, sharing *sharings.Sharing) error {
	requestPerm, err := perm.GetPermission(c)
	if err != nil {
		return err
	}

	if sharing.Owner && sharing.Permissions.IsSubSetOf(requestPerm.Permissions) {
		return nil
	}

	return sharings.ErrForbidden
}

func checkRevokeRecipientPermissions(c echo.Context, sharing *sharings.Sharing, recipientClientID string) error {
	requestPerm, err := perm.GetPermission(c)
	if err != nil {
		return err
	}

	if requestPerm.Type != permissions.TypeOauth {
		return sharings.ErrForbidden
	}

	if !sharing.Permissions.HasSameRules(requestPerm.Permissions) {
		return permissions.ErrInvalidToken
	}

	if sharing.Owner {
		for _, rec := range sharing.RecipientsStatus {
			if rec.Client.ClientID == recipientClientID {
				if requestPerm.SourceID == rec.HostClientID {
					return nil
				}
			}
		}
	} else {
		sharerClientID := sharing.Sharer.SharerStatus.HostClientID
		if requestPerm.SourceID == sharerClientID {
			return nil
		}
	}

	return sharings.ErrForbidden
}

// Check if the permissions given in the revoke request apply.
//
// Two scenarii can lead to valid permissions:
// 1. The permissions identify the application
// 2. The permissions identify the user that is to be revoked or the sharer.
func checkRevokeSharingPermissions(c echo.Context, sharing *sharings.Sharing) (string, error) {
	requestPerm, err := perm.GetPermission(c)
	if err != nil {
		return "", err
	}

	switch requestPerm.Type {
	case permissions.TypeWebapp:
		if sharing.Permissions.IsSubSetOf(requestPerm.Permissions) {
			return requestPerm.Type, nil
		}
		return "", sharings.ErrForbidden

	case permissions.TypeOauth:
		if !sharing.Permissions.HasSameRules(requestPerm.Permissions) {
			return "", permissions.ErrInvalidToken
		}
		if !sharing.Owner {
			sharerClientID := sharing.Sharer.SharerStatus.HostClientID
			if requestPerm.SourceID == sharerClientID {
				return requestPerm.Type, nil
			}
		}
		return "", sharings.ErrForbidden
	}

	return "", permissions.ErrInvalidAudience
}

// checkCreatePermissions checks the sharer's token has all the permissions
// matching the ones defined in the sharing document
func checkCreatePermissions(c echo.Context, sharing *sharings.Sharing) error {
	requestPerm, err := perm.GetPermission(c)
	if err != nil {
		return err
	}
	if !sharing.Permissions.IsSubSetOf(requestPerm.Permissions) {
		return echo.NewHTTPError(http.StatusForbidden)
	}
	return nil
}

// checkGetPermissions checks the requester's token has at least one doctype
// permission declared in the sharing document
func checkGetPermissions(c echo.Context, sharing *sharings.Sharing) error {
	requestPerm, err := perm.GetPermission(c)
	if err != nil {
		return err
	}
	for _, rule := range sharing.Permissions {
		if requestPerm.Permissions.RuleInSubset(rule) {
			return nil
		}
	}
	return echo.NewHTTPError(http.StatusForbidden)
}

// wrapErrors returns a formatted error
func wrapErrors(err error) error {
	switch err {
	case sharings.ErrBadSharingType:
		return jsonapi.InvalidParameter("sharing_type", err)
	case sharings.ErrRecipientDoesNotExist:
		return jsonapi.NotFound(err)
	case sharings.ErrMissingScope, sharings.ErrMissingState, sharings.ErrRecipientHasNoURL,
		sharings.ErrRecipientHasNoEmail, sharings.ErrRecipientBadParams:
		return jsonapi.BadRequest(err)
	case sharings.ErrSharingDoesNotExist, sharings.ErrPublicNameNotDefined:
		return jsonapi.NotFound(err)
	case sharings.ErrMailCouldNotBeSent:
		return jsonapi.InternalServerError(err)
	case sharings.ErrNoOAuthClient:
		return jsonapi.BadRequest(err)
	}
	return err
}

func doctypeExists(ins *instance.Instance, doctype string) bool {
	_, err := couchdb.DBStatus(ins, doctype)
	return err == nil
}
