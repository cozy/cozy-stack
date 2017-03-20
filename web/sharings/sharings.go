package sharings

import (
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/sharings"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

// SharingAnswer handles a sharing answer from the sharer side
func SharingAnswer(c echo.Context) error {
	var err error
	var u string

	state := c.QueryParam("state")
	clientID := c.QueryParam("client_id")
	accessCode := c.QueryParam("access_code")

	instance := middlewares.GetInstance(c)

	// The sharing is refused if there is no access code
	sharingAccepted := accessCode != ""

	if sharingAccepted {
		u, err = sharings.SharingAccepted(instance, state, clientID, accessCode)
	} else {
		u, err = sharings.SharingRefused(instance, state, clientID)
	}
	if err != nil {
		return wrapErrors(err)
	}
	return c.Redirect(http.StatusFound, u)
}

// AddRecipient adds a sharing Recipient and register to its server
func AddRecipient(c echo.Context) error {

	recipient := new(sharings.Recipient)
	if err := c.Bind(recipient); err != nil {
		return err
	}
	instance := middlewares.GetInstance(c)

	err := sharings.CreateRecipient(instance, recipient)
	if err != nil {
		return wrapErrors(err)
	}

	err = recipient.Register(instance)
	if err != nil {
		return wrapErrors(err)
	}

	return jsonapi.Data(c, http.StatusCreated, recipient, nil)
}

// SharingRequest handles a sharing request from the recipient side
// It creates a tempory sharing document and redirect to the authorize page
func SharingRequest(c echo.Context) error {
	scope := c.QueryParam("scope")
	state := c.QueryParam("state")
	sharingType := c.QueryParam("sharing_type")
	desc := c.QueryParam("desc")

	instance := middlewares.GetInstance(c)

	_, err := sharings.CreateSharingRequest(instance, desc, state, sharingType, scope)
	if err != nil {
		return wrapErrors(err)
	}

	redirectAuthorize := instance.PageURL("/auth/authorize", c.QueryParams())
	return c.Redirect(http.StatusSeeOther, redirectAuthorize)
}

// CreateSharing initializes a sharing by creating the associated document
func CreateSharing(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	sharing := new(sharings.Sharing)
	if err := c.Bind(sharing); err != nil {
		return err
	}

	if err := sharings.CheckSharingCreation(instance, sharing); err != nil {
		return wrapErrors(err)
	}

	err := sharings.Create(instance, sharing)
	if err != nil {
		return err
	}

	err = sharings.SendSharingMails(instance, sharing)
	if err != nil {
		return wrapErrors(err)
	}

	return jsonapi.Data(c, http.StatusCreated, sharing, nil)
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

// RecipientRefusedSharing is called when the recipient refused the sharing.
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

	return sharings.RecipientRefusedSharing(instance, sharingID, clientID)
}

// Routes sets the routing for the sharing service
func Routes(router *echo.Group) {
	router.POST("/", CreateSharing)
	router.PUT("/:id/sendMails", SendSharingMails)
	router.GET("/request", SharingRequest)
	router.POST("/answer", SharingAnswer)
	router.POST("/formRefuse", RecipientRefusedSharing)
	router.GET("/answer", SharingAnswer)
	router.POST("/recipient", AddRecipient)
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
