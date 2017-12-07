package sharings

import (
	"net/http"

	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/sharings"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	perm "github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

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

func checkRevokeContactPermissions(c echo.Context, sharing *sharings.Sharing) error {
	ins := middlewares.GetInstance(c)
	sharingPerms, err := sharing.Permissions(ins)
	if err != nil {
		return err
	}

	requestPerm, err := perm.GetPermission(c)
	if err != nil {
		return err
	}

	if sharing.Owner && sharingPerms.Permissions.IsSubSetOf(requestPerm.Permissions) {
		return nil
	}

	return sharings.ErrForbidden
}

func checkRevokeRecipientPermissions(c echo.Context, sharing *sharings.Sharing, recipientClientID string) error {
	ins := middlewares.GetInstance(c)
	sharingPerms, err := sharing.Permissions(ins)
	if err != nil {
		return err
	}

	requestPerm, err := perm.GetPermission(c)
	if err != nil {
		return err
	}

	if requestPerm.Type != permissions.TypeOauth {
		return sharings.ErrForbidden
	}

	if !sharingPerms.Permissions.HasSameRules(requestPerm.Permissions) {
		return permissions.ErrInvalidToken
	}

	if sharing.Owner {
		for _, rec := range sharing.Recipients {
			if rec.Client.ClientID == recipientClientID {
				if requestPerm.SourceID == rec.InboundClientID {
					return nil
				}
			}
		}
	} else {
		sharerClientID := sharing.Sharer.InboundClientID
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
	ins := middlewares.GetInstance(c)
	sharingPerms, err := sharing.Permissions(ins)
	if err != nil {
		return "", err
	}

	requestPerm, err := perm.GetPermission(c)
	if err != nil {
		return "", err
	}

	switch requestPerm.Type {
	case permissions.TypeWebapp:
		if sharingPerms.Permissions.IsSubSetOf(requestPerm.Permissions) {
			return requestPerm.Type, nil
		}
		return "", sharings.ErrForbidden

	case permissions.TypeOauth:
		if !sharingPerms.Permissions.HasSameRules(requestPerm.Permissions) {
			return "", permissions.ErrInvalidToken
		}
		if !sharing.Owner {
			sharerClientID := sharing.Sharer.InboundClientID
			if requestPerm.SourceID == sharerClientID {
				return requestPerm.Type, nil
			}
		}
		return "", sharings.ErrForbidden
	}

	return "", permissions.ErrInvalidAudience
}
