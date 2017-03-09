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

// Routes sets the routing for the sharing service
func Routes(router *echo.Group) {
	router.POST("/", CreateSharing)
	router.PUT("/:id/sendMails", SendSharingMails)
	router.GET("/request", SharingRequest)
}

// wrapErrors returns a formatted error
func wrapErrors(err error) error {
	switch err {
	case sharings.ErrBadSharingType:
		return jsonapi.InvalidParameter("sharing_type", err)
	case sharings.ErrRecipientDoesNotExist:
		return jsonapi.NotFound(err)
	case sharings.ErrMissingScope:
		return jsonapi.BadRequest(err)
	case sharings.ErrMissingState:
		return jsonapi.BadRequest(err)
	case sharings.ErrSharingDoesNotExist:
		return jsonapi.NotFound(err)
	case sharings.ErrMailCouldNotBeSent:
		return jsonapi.InternalServerError(err)
	}
	return err
}
