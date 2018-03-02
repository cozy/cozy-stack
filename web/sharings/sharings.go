package sharings

import (
	"errors"
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/pkg/contacts"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/sharing"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	perm "github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

type apiSharing struct {
	*sharing.Sharing
}

func (s *apiSharing) Included() []jsonapi.Object             { return nil }
func (s *apiSharing) Relationships() jsonapi.RelationshipMap { return nil }
func (s *apiSharing) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/sharings/" + s.SID}
}

var _ jsonapi.Object = (*apiSharing)(nil)

// CreateSharing initializes a new sharing
func CreateSharing(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	var s sharing.Sharing
	obj, err := jsonapi.Bind(c.Request(), &s)
	if err != nil {
		return jsonapi.BadJSON()
	}

	slug, err := checkCreatePermissions(c, &s)
	if err != nil {
		return echo.NewHTTPError(http.StatusForbidden)
	}

	if err := s.BeOwner(inst, slug); err != nil {
		return wrapErrors(err)
	}

	if rel, ok := obj.GetRelationship("recipients"); ok {
		if data, ok := rel.Data.([]interface{}); ok {
			for _, ref := range data {
				if id, ok := ref.(map[string]interface{})["id"].(string); ok {
					if err := s.AddContact(inst, id); err != nil {
						return err
					}
				}
			}
		}
	}

	if err := s.Create(inst); err != nil {
		return wrapErrors(err)
	}
	return jsonapi.Data(c, http.StatusCreated, &apiSharing{&s}, nil)
}

// Routes sets the routing for the sharing service
func Routes(router *echo.Group) {
	router.POST("/", CreateSharing)
}

func extractSlugFromSourceID(sourceID string) (string, error) {
	parts := strings.SplitN(sourceID, "/", 2)
	if len(parts) < 2 {
		return "", jsonapi.BadRequest(errors.New("Invalid request"))
	}
	slug := parts[1]
	return slug, nil
}

// checkCreatePermissions checks the sharer's token has all the permissions
// matching the ones defined in the sharing document
func checkCreatePermissions(c echo.Context, s *sharing.Sharing) (string, error) {
	requestPerm, err := perm.GetPermission(c)
	if err != nil {
		return "", err
	}
	if requestPerm.Type != permissions.TypeWebapp &&
		requestPerm.Type != permissions.TypeOauth {
		return "", permissions.ErrInvalidAudience
	}
	// TODO add tests
	for _, r := range s.Rules {
		pr := permissions.Rule{
			Title:    r.Title,
			Type:     r.DocType,
			Verbs:    permissions.ALL,
			Selector: r.Selector,
			Values:   r.Values,
		}
		if !requestPerm.Permissions.RuleInSubset(pr) {
			return "", echo.NewHTTPError(http.StatusForbidden)
		}
	}
	if requestPerm.Type == permissions.TypeOauth {
		return "", nil
	}
	return extractSlugFromSourceID(requestPerm.SourceID)
}

// wrapErrors returns a formatted error
func wrapErrors(err error) error {
	switch err {
	case contacts.ErrNoMailAddress:
		return jsonapi.InvalidAttribute("recipients", err)
	case sharing.ErrNoRecipients, sharing.ErrNoRules:
		return jsonapi.BadRequest(err)
	}
	return err
}
