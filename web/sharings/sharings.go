package sharings

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/sharings"
	"github.com/cozy/cozy-stack/web/data"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

type apiSharing struct {
	*sharings.Sharing
}

func (s *apiSharing) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Sharing)
}
func (s *apiSharing) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/sharings/" + s.SID}
}

// Relationships is part of the jsonapi.Object interface
// It is used to generate the recipients relationships
func (s *apiSharing) Relationships() jsonapi.RelationshipMap {
	l := len(s.RecipientsStatus)
	i := 0

	data := make([]couchdb.DocReference, l)
	for _, rec := range s.RecipientsStatus {
		r := rec.RefRecipient
		data[i] = couchdb.DocReference{ID: r.ID, Type: r.Type}
		i++
	}
	contents := jsonapi.Relationship{Data: data}
	return jsonapi.RelationshipMap{"recipients": contents}
}

// Included is part of the jsonapi.Object interface
func (s *apiSharing) Included() []jsonapi.Object {
	var included []jsonapi.Object
	for _, rec := range s.RecipientsStatus {
		r := rec.GetCachedRecipient()
		included = append(included, &apiRecipient{r})
	}
	return included
}

type apiRecipient struct {
	*sharings.Recipient
}

func (r *apiRecipient) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.Recipient)
}

func (r *apiRecipient) Relationships() jsonapi.RelationshipMap { return nil }
func (r *apiRecipient) Included() []jsonapi.Object             { return nil }
func (r *apiRecipient) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/recipients/" + r.RID}
}

var _ jsonapi.Object = (*apiSharing)(nil)
var _ jsonapi.Object = (*apiRecipient)(nil)

// SharingAnswer handles a sharing answer from the sharer side
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

// CreateRecipient adds a sharing Recipient.
func CreateRecipient(c echo.Context) error {

	recipient := new(sharings.Recipient)
	if err := c.Bind(recipient); err != nil {
		return err
	}
	instance := middlewares.GetInstance(c)

	err := sharings.CreateRecipient(instance, recipient)
	if err != nil {
		return wrapErrors(err)
	}

	return jsonapi.Data(c, http.StatusCreated, &apiRecipient{recipient}, nil)
}

// SharingRequest handles a sharing request from the recipient side.
// It creates a temporary sharing document and redirects to the authorize page.
func SharingRequest(c echo.Context) error {
	scope := c.QueryParam("scope")
	state := c.QueryParam("state")
	sharingType := c.QueryParam("sharing_type")
	desc := c.QueryParam("desc")
	clientID := c.QueryParam("client_id")

	instance := middlewares.GetInstance(c)

	sharing, err := sharings.CreateSharingRequest(instance, desc, state, sharingType, scope, clientID)
	if err != nil {
		return wrapErrors(err)
	}
	// Particular case for master-master: register the sharer
	if sharingType == consts.MasterMasterSharing {
		if err = sharings.RegisterSharer(instance, sharing); err != nil {
			return err
		}
		if err = sharings.SendClientID(instance, sharing); err != nil {
			return err
		}
	}

	redirectAuthorize := instance.PageURL("/auth/authorize", c.QueryParams())
	return c.Redirect(http.StatusSeeOther, redirectAuthorize)
}

// CreateSharing initializes a sharing by creating the associated document,
// registering the sharer as a new OAuth client at each recipient as well as
// sending them a mail invitation.
func CreateSharing(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	sharing := new(sharings.Sharing)
	if err := c.Bind(sharing); err != nil {
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

// SendSharingMails sends the mails requests for the provided sharing.
func SendSharingMails(c echo.Context) error {
	// Fetch the instance.
	instance := middlewares.GetInstance(c)

	// Fetch the document id and then the sharing document.
	docID := c.Param("id")
	sharing := &sharings.Sharing{}
	err := couchdb.GetDoc(instance, consts.Sharings, docID, sharing)
	if err != nil {
		err = sharings.ErrSharingDoesNotExist
		return wrapErrors(err)
	}

	// Send the mails.
	err = sharings.SendSharingMails(instance, sharing)
	if err != nil {
		return wrapErrors(err)
	}

	return nil
}

// AddSharingRecipient adds an existing recipient to an existing sharing
func AddSharingRecipient(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	// Get sharing doc
	id := c.Param("id")
	sharing := &sharings.Sharing{}
	err := couchdb.GetDoc(instance, consts.Sharings, id, sharing)
	if err != nil {
		err = sharings.ErrSharingDoesNotExist
		return wrapErrors(err)
	}

	// Create recipient, register, and send mail
	ref := couchdb.DocReference{}
	if err = c.Bind(&ref); err != nil {
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
	fmt.Printf("add recipient ok\n")
	fmt.Printf("sharing : %+v\n", sharing)

	return jsonapi.Data(c, http.StatusOK, &apiSharing{sharing}, nil)

}

// RecipientRefusedSharing is called when the recipient refused the sharing.
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

// ReceiveClientID receives an OAuth ClientID in a master-master context.
// This is called from a recipient, after he registered himself to the sharer.
// The received clientID is called a HostClientID, as it refers to a client
// created by the sharer, i.e. the host here.
func ReceiveClientID(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	p := &sharings.SharingRequestParams{}
	if err := c.Bind(p); err != nil {
		return err
	}
	sharing, rec, err := sharings.FindSharingRecipient(instance, p.SharingID, p.ClientID)
	if err != nil {
		return err
	}
	rec.HostClientID = p.HostClientID
	err = couchdb.UpdateDoc(instance, sharing)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, nil)
}

// receiveDocument stores a shared document in the Cozy.
//
// If the document to store is a "io.cozy.files" our custom handler will be
// called, otherwise we will redirect to /data.
func receiveDocument(c echo.Context) error {
	var err error

	switch c.Param("doctype") {
	case consts.Files:
		err = creationWithIDHandler(c)
	default:
		err = data.UpdateDoc(c)
	}

	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, nil)
}

// updateDocument updates a shared document in the Cozy.
//
// TODO Handle files updates.
func updateDocument(c echo.Context) error {
	var err error

	switch c.Param("doctype") {
	case consts.Files:
		// TODO
		err = c.JSON(http.StatusNotImplemented, nil)
	default:
		err = data.UpdateDoc(c)
	}

	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, nil)
}

// deleteDocument deletes the shared document from the Cozy.
//
// TODO Handle files deletions.
func deleteDocument(c echo.Context) error {
	var err error

	switch c.Param("doctype") {
	case consts.Files:
		// TODO
		err = c.JSON(http.StatusNotImplemented, nil)
	default:
		err = data.DeleteDoc(c)
	}

	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, nil)
}

// Routes sets the routing for the sharing service
func Routes(router *echo.Group) {
	router.POST("/", CreateSharing)
	router.PUT("/:id/recipient", AddSharingRecipient)
	router.PUT("/:id/sendMails", SendSharingMails)
	router.GET("/request", SharingRequest)
	router.GET("/answer", SharingAnswer)
	router.POST("/formRefuse", RecipientRefusedSharing)
	router.POST("/recipient", CreateRecipient)
	router.POST("/access/client", ReceiveClientID)

	group := router.Group("/doc/:doctype", data.ValidDoctype)
	group.POST("/:docid", receiveDocument)
	group.PUT("/:docid", updateDocument)
	group.DELETE("/:docid", deleteDocument)
}

// wrapErrors returns a formatted error
func wrapErrors(err error) error {
	switch err {
	case sharings.ErrBadSharingType:
		return jsonapi.InvalidParameter("sharing_type", err)
	case sharings.ErrRecipientDoesNotExist:
		return jsonapi.NotFound(err)
	case sharings.ErrMissingScope, sharings.ErrMissingState, sharings.ErrRecipientHasNoURL,
		sharings.ErrRecipientHasNoEmail:
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
