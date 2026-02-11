package sharings

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// RevokeSharing is used to revoke a sharing by the sharer, for all recipients
func RevokeSharing(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}
	_, err = checkCreatePermissions(c, s)
	if err != nil {
		return echo.NewHTTPError(http.StatusForbidden)
	}
	if err = s.Revoke(inst); err != nil {
		return wrapErrors(err)
	}
	return c.NoContent(http.StatusNoContent)
}

// RevokeRecipient is used by the owner to revoke a recipient
func RevokeRecipient(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}
	_, err = checkCreatePermissions(c, s)
	if err != nil {
		return echo.NewHTTPError(http.StatusForbidden)
	}
	index, err := strconv.Atoi(c.Param("index"))
	if err != nil {
		return jsonapi.InvalidParameter("index", err)
	}
	if index == 0 || index >= len(s.Members) {
		return jsonapi.InvalidParameter("index", errors.New("Invalid index"))
	}
	if err = s.RevokeRecipient(inst, index); err != nil {
		return wrapErrors(err)
	}
	go s.NotifyRecipients(inst, nil)
	return c.NoContent(http.StatusNoContent)
}

// RevokeGroup is used by the owner to revoke a group
func RevokeGroup(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}
	_, err = checkCreatePermissions(c, s)
	if err != nil {
		return echo.NewHTTPError(http.StatusForbidden)
	}
	index, err := strconv.Atoi(c.Param("index"))
	if err != nil {
		return jsonapi.InvalidParameter("index", err)
	}
	if index >= len(s.Groups) {
		return jsonapi.InvalidParameter("index", errors.New("Invalid index"))
	}
	if err = s.RevokeGroup(inst, index); err != nil {
		return wrapErrors(err)
	}
	go s.NotifyRecipients(inst, nil)
	return c.NoContent(http.StatusNoContent)
}

// getBearerToken extracts the Bearer token from the Authorization header
func getBearerToken(c echo.Context) string {
	token := c.Request().Header.Get(echo.HeaderAuthorization)
	return strings.TrimPrefix(token, "Bearer ")
}

// checkDriveSharingWritePermissions checks if the request has valid permissions for a Drive sharing.
// For non-Drive sharings, it falls back to OAuth-based permission checking.
func checkDriveSharingWritePermissions(c echo.Context, s *sharing.Sharing) error {
	if s.Drive {
		token := getBearerToken(c)
		if token == "" || len(s.Credentials) == 0 || token != s.Credentials[0].DriveToken {
			return echo.NewHTTPError(http.StatusForbidden)
		}
		return nil
	}
	return hasSharingWritePermissions(c)
}

// RevocationRecipientNotif is used to inform a recipient that the sharing is revoked
func RevocationRecipientNotif(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}
	if err := checkDriveSharingWritePermissions(c, s); err != nil {
		return err
	}
	if err = s.RevokeByNotification(inst); err != nil {
		return wrapErrors(err)
	}
	return c.NoContent(http.StatusNoContent)
}

// RevocationOwnerNotif is used to inform the owner that a recipient has revoked
// himself/herself from the sharing
func RevocationOwnerNotif(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}

	// For Drive sharings, FindMemberByInteractCode validates the token and finds the member.
	// For non-Drive sharings, we check permissions first, then get the member via OAuth.
	var member *sharing.Member
	if s.Drive {
		token := getBearerToken(c)
		if token == "" {
			return echo.NewHTTPError(http.StatusForbidden)
		}
		member, err = s.FindMemberByInteractCode(inst, token)
	} else {
		if err := hasSharingWritePermissions(c); err != nil {
			return err
		}
		member, err = requestMember(c, s)
	}
	if err != nil {
		return wrapErrors(err)
	}

	if err = s.RevokeRecipientByNotification(inst, member); err != nil {
		return wrapErrors(err)
	}
	go s.NotifyRecipients(inst, nil)
	return c.NoContent(http.StatusNoContent)
}

// RevokeRecipientBySelf is used by a recipient to revoke himself/herself
// from the sharing
func RevokeRecipientBySelf(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}
	_, err = checkCreatePermissions(c, s)
	if err != nil {
		return echo.NewHTTPError(http.StatusForbidden)
	}
	if err = s.RevokeRecipientBySelf(inst, sharing.SharingDirNotTrashed); err != nil {
		return wrapErrors(err)
	}
	return c.NoContent(http.StatusNoContent)
}
