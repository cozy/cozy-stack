package sharings

import (
	"net/http"

	"github.com/cozy/cozy-stack/pkg/sharings"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

// SharingRequest handles a sharing request from the recipient side
// It creates a tempory sharing document, waiting for the recipient answer
func SharingRequest(c echo.Context) error {
	scope := c.QueryParam("scope")
	state := c.QueryParam("state")
	sharingType := c.QueryParam("sharing_type")

	instance := middlewares.GetInstance(c)

	sharing, err := sharings.CreateSharingRequest(instance, state, sharingType, scope)
	if err != nil {
		return wrapErrors(err)
	}

	//TODO call the OAuth authorize to display the permissions
	//TODO return the permission html

	return jsonapi.Data(c, http.StatusCreated, sharing, nil)
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

	return jsonapi.Data(c, http.StatusCreated, sharing, nil)
}

// Routes sets the routing for the sharing service
func Routes(router *echo.Group) {
	// API Routes
	router.POST("/", CreateSharing)
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
	}
	return err
}
