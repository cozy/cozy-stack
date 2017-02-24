package sharings

import (
	"fmt"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/sharings"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

// SharingRequest handles a sharing request from the recipient side
// It creates a tempory sharing document, waiting for the recipient answer
func SharingRequest(c echo.Context) error {
	//Get the permissions and sharing id
	scope := c.QueryParam("scope")
	if scope == "" {
		return wrapErrors(sharings.ErrMissingScope)
	}
	state := c.QueryParam("state")
	if state == "" {
		return wrapErrors(sharings.ErrMissingState)
	}
	sharingType := c.QueryParam("sharing_type")
	//TODO : Check sharing type integrity
	if sharingType == "" {
		return wrapErrors(sharings.ErrBadSharingType)
	}
	fmt.Printf("scope : %v\n", scope)
	permissions, err := permissions.UnmarshalScopeString(scope)
	if err != nil {
		fmt.Println("error...")
		return err
	}
	fmt.Printf("perm : %+v", permissions)

	sharing := &sharings.Sharing{
		SharingType: sharingType,
		SharingID:   state,
		Permissions: permissions,
		Owner:       false,
	}

	instance := middlewares.GetInstance(c)
	_, err = sharings.Create(instance, sharing)
	if err != nil {
		return err
	}

	fmt.Printf("state : %v", sharing.SharingID)

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
