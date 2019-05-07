package sharings

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
)

// AddReadOnly is used to downgrade a read-write member to read-only
func AddReadOnly(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}
	_, err = checkCreatePermissions(c, s)
	if err != nil {
		// It can be a delegated call from a member on an open sharing to the owner
		if err = hasSharingWritePermissions(c); err != nil {
			return err
		}
	}
	index, err := strconv.Atoi(c.Param("index"))
	if err != nil {
		return jsonapi.InvalidParameter("index", err)
	}
	if index == 0 || index >= len(s.Members) {
		return jsonapi.InvalidParameter("index", errors.New("Invalid index"))
	}
	if s.Owner {
		if err = s.AddReadOnlyFlag(inst, index); err != nil {
			return wrapErrors(err)
		}
		go s.NotifyRecipients(inst, nil)
	} else {
		if err = s.DelegateAddReadOnlyFlag(inst, index); err != nil {
			return wrapErrors(err)
		}
	}
	return c.NoContent(http.StatusNoContent)
}

// DowngradeToReadOnly is used to receive the credentials for pushing last changes
// on an instance of a recipient before going to read-only mode
func DowngradeToReadOnly(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}
	var creds sharing.APICredentials
	if _, err = jsonapi.Bind(c.Request().Body, &creds); err != nil {
		return jsonapi.BadJSON()
	}
	if err = s.DowngradeToReadOnly(inst, &creds); err != nil {
		return wrapErrors(err)
	}
	return c.NoContent(http.StatusNoContent)
}

// RemoveReadOnly is used to give read-write to a member that had the read-only flag
func RemoveReadOnly(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}
	_, err = checkCreatePermissions(c, s)
	if err != nil {
		// It can be a delegated call from a member on an open sharing to the owner
		if err = hasSharingWritePermissions(c); err != nil {
			return err
		}
	}
	index, err := strconv.Atoi(c.Param("index"))
	if err != nil {
		return jsonapi.InvalidParameter("index", err)
	}
	if index == 0 || index >= len(s.Members) {
		return jsonapi.InvalidParameter("index", errors.New("Invalid index"))
	}
	if s.Owner {
		if err = s.RemoveReadOnlyFlag(inst, index); err != nil {
			return wrapErrors(err)
		}
		go s.NotifyRecipients(inst, nil)
	} else {
		if err = s.DelegateRemoveReadOnlyFlag(inst, index); err != nil {
			return wrapErrors(err)
		}
	}
	return c.NoContent(http.StatusNoContent)
}

// UpgradeToReadWrite is used to receive the credentials for pushing updates on
// an instance of a recipient that was in read-only mode
func UpgradeToReadWrite(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}
	var creds sharing.APICredentials
	if _, err = jsonapi.Bind(c.Request().Body, &creds); err != nil {
		return jsonapi.BadJSON()
	}
	if err = s.UpgradeToReadWrite(inst, &creds); err != nil {
		return wrapErrors(err)
	}
	return c.NoContent(http.StatusNoContent)
}
