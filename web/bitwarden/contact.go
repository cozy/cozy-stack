package bitwarden

import (
	"net/http"

	"github.com/cozy/cozy-stack/model/bitwarden"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/hashicorp/go-multierror"
	"github.com/labstack/echo/v4"
)

// RefuseContact is the API handler for DELETE /bitwarden/contacts/:id. It is
// used for refusing to give access to a user to shared ciphers, and removes
// them from all the sharings.
func RefuseContact(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.DELETE, consts.BitwardenContacts); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid token",
		})
	}

	id := c.Param("id")
	var contact bitwarden.Contact
	if err := couchdb.GetDoc(inst, consts.BitwardenContacts, id, &contact); err != nil {
		if couchdb.IsNotFoundError(err) {
			return c.JSON(http.StatusNotFound, echo.Map{
				"error": "not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}
	email := contact.Email
	if err := couchdb.DeleteDoc(inst, &contact); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	sharings, err := sharing.GetSharingsByDocType(inst, consts.BitwardenOrganizations)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}
	var errm error
	for _, s := range sharings {
		if !s.Owner {
			continue
		}
		for i, m := range s.Members {
			if i != 0 && m.Email == email && m.Status == sharing.MemberStatusReady {
				if err := s.RevokeRecipient(inst, i); err != nil {
					errm = multierror.Append(errm, err)
				}
			}
		}
	}
	if errm != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	return c.NoContent(http.StatusNoContent)
}
