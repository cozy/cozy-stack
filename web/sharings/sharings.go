package sharings

import (
	"net/http"

	"github.com/cozy/cozy-stack/pkg/sharings"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

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

	doc, err := sharings.Create(instance, sharing)
	if err != nil {
		return err
	}

	return jsonapi.Data(c, http.StatusOK, doc, nil)
}

// wrapErrors returns a formatted error
func wrapErrors(err error) error {
	switch err {
	case sharings.ErrBadSharingType:
		return jsonapi.InvalidParameter("sharing_type", err)
	case sharings.ErrRecipientDoesNotExist:
		return jsonapi.NotFound(err)
	}
	return err
}

// Routes sets the routing for the sharing service
func Routes(router *echo.Group) {
	// API Routes
	router.POST("/", CreateSharing)
}
