package permissions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
	"github.com/justincampbell/bigduration"
)

// ErrPatchCodeOrSet is returned when an attempt is made to patch both
// code & set in one request
var ErrPatchCodeOrSet = echo.NewHTTPError(http.StatusBadRequest,
	"The patch doc should have property 'codes' or 'permissions', not both")

// ContextPermissionSet is the key used in echo context to store permissions set
const ContextPermissionSet = "permissions_set"

// ContextClaims is the key used in echo context to store claims
const ContextClaims = "token_claims"

// APIPermission is the struct that will be used to serialized a permission to
// JSON-API
type APIPermission struct {
	*permission.Permission
}

// MarshalJSON implements jsonapi.Doc
func (p *APIPermission) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.Permission)
}

// Relationships implements jsonapi.Doc
func (p *APIPermission) Relationships() jsonapi.RelationshipMap { return nil }

// Included implements jsonapi.Doc
func (p *APIPermission) Included() []jsonapi.Object { return nil }

// Links implements jsonapi.Doc
func (p *APIPermission) Links() *jsonapi.LinksList {
	links := &jsonapi.LinksList{Self: "/permissions/" + p.PID}
	parts := strings.SplitN(p.SourceID, "/", 2)
	if parts[0] == consts.Sharings {
		links.Related = "/sharings/" + parts[1]
	}
	return links
}

type getPermsFunc func(db prefixer.Prefixer, id string) (*permission.Permission, error)

func displayPermissions(c echo.Context) error {
	doc, err := middlewares.GetPermission(c)
	if err != nil {
		return err
	}
	doc.Codes = nil // XXX hides the codes in the response
	return jsonapi.Data(c, http.StatusOK, &APIPermission{doc}, nil)
}

func createPermission(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	names := strings.Split(c.QueryParam("codes"), ",")
	ttl := c.QueryParam("ttl")

	parent, err := middlewares.GetPermission(c)
	if err != nil {
		return err
	}

	sourceID := parent.SourceID
	// Check if the permission is linked to an OAuth Client
	if parent.Client != nil {
		oauthClient := parent.Client.(*oauth.Client)
		if auth.IsLinkedApp(oauthClient.SoftwareID) {
			slug := auth.GetLinkedAppSlug(oauthClient.SoftwareID)
			// Changing the sourceID from the OAuth clientID to the classic
			// io.cozy.apps/slug one
			sourceID = consts.Apps + "/" + slug
		}
	}

	var subdoc permission.Permission
	if _, err = jsonapi.Bind(c.Request().Body, &subdoc); err != nil {
		return err
	}

	var codes map[string]string
	var shortcodes map[string]string

	if names != nil {
		codes = make(map[string]string, len(names))
		shortcodes = make(map[string]string, len(names))
		for _, name := range names {
			longcode, err := instance.CreateShareCode(name)
			shortcode := crypto.GenerateRandomString(consts.ShortCodeLen)

			codes[name] = longcode
			shortcodes[name] = shortcode
			if err != nil {
				return err
			}
		}
	}

	if parent == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "no parent")
	}

	var expiresAt *time.Time
	if ttl != "" {
		if d, errd := bigduration.ParseDuration(ttl); errd == nil {
			ex := time.Now().Add(d)
			expiresAt = &ex
		}
	}

	pdoc, err := permission.CreateShareSet(instance, parent, sourceID, codes, shortcodes, subdoc.Permissions, expiresAt)
	if err != nil {
		return err
	}

	return jsonapi.Data(c, http.StatusOK, &APIPermission{pdoc}, nil)
}

const (
	defaultPermissionsByDoctype = 30
	maxPermissionsByDoctype     = 100
)

func listPermissionsByDoctype(c echo.Context, route, permType string) error {
	ins := middlewares.GetInstance(c)
	doctype := c.Param("doctype")
	if doctype == "" {
		return jsonapi.NewError(http.StatusBadRequest, "Missing doctype")
	}

	current, err := middlewares.GetPermission(c)
	if err != nil {
		return err
	}

	if !current.Permissions.AllowWholeType(http.MethodGet, doctype) {
		return jsonapi.NewError(http.StatusForbidden,
			"You need GET permission on whole type to list its permissions")
	}

	cursor, err := jsonapi.ExtractPaginationCursor(c, defaultPermissionsByDoctype, maxPermissionsByDoctype)
	if err != nil {
		return err
	}

	perms, err := permission.GetPermissionsByDoctype(ins, permType, doctype, cursor)
	if err != nil {
		return err
	}

	links := &jsonapi.LinksList{}
	if cursor.HasMore() {
		params, err := jsonapi.PaginationCursorToParams(cursor)
		if err != nil {
			return err
		}
		links.Next = fmt.Sprintf("/permissions/doctype/%s/%s?%s",
			doctype, route, params.Encode())
	}

	out := make([]jsonapi.Object, len(perms))
	for i := range perms {
		out[i] = &APIPermission{&perms[i]}
	}

	return jsonapi.DataList(c, http.StatusOK, out, links)
}

func listByLinkPermissionsByDoctype(c echo.Context) error {
	return listPermissionsByDoctype(c, "shared-by-link", permission.TypeShareByLink)
}

type refAndVerb struct {
	ID      string              `json:"id"`
	DocType string              `json:"type"`
	Verbs   *permission.VerbSet `json:"verbs"`
}

func listPermissions(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	references, err := jsonapi.BindRelations(c.Request())
	if err != nil {
		return err
	}
	ids := make(map[string][]string)
	for _, ref := range references {
		idSlice, ok := ids[ref.Type]
		if !ok {
			idSlice = []string{}
		}
		ids[ref.Type] = append(idSlice, ref.ID)
	}

	var out []refAndVerb
	for doctype, idSlice := range ids {
		result, err2 := permission.GetPermissionsForIDs(instance, doctype, idSlice)
		if err2 != nil {
			return err2
		}
		for id, verbs := range result {
			out = append(out, refAndVerb{id, doctype, verbs})
		}
	}

	data, err := json.Marshal(out)
	if err != nil {
		return err
	}
	doc := jsonapi.Document{
		Data: (*json.RawMessage)(&data),
	}
	resp := c.Response()
	resp.Header().Set("Content-Type", jsonapi.ContentType)
	resp.WriteHeader(http.StatusOK)
	return json.NewEncoder(resp).Encode(doc)
}

func patchPermission(getPerms getPermsFunc, paramName string) echo.HandlerFunc {
	return func(c echo.Context) error {
		instance := middlewares.GetInstance(c)
		current, err := middlewares.GetPermission(c)
		if err != nil {
			return err
		}

		var patch permission.Permission
		if _, err = jsonapi.Bind(c.Request().Body, &patch); err != nil {
			return err
		}

		patchSet := patch.Permissions != nil && len(patch.Permissions) > 0
		patchCodes := len(patch.Codes) > 0

		if patchCodes == patchSet {
			return ErrPatchCodeOrSet
		}

		toPatch, err := getPerms(instance, c.Param(paramName))
		if err != nil {
			return err
		}

		if patchCodes {
			if !current.ParentOf(toPatch) {
				return permission.ErrNotParent
			}
			toPatch.PatchCodes(patch.Codes)
		}

		if patchSet {
			for _, r := range patch.Permissions {
				if r.Type == "" {
					toPatch.RemoveRule(r)
				} else if current.Permissions.RuleInSubset(r) {
					toPatch.AddRules(r)
				} else {
					return permission.ErrNotSubset
				}
			}
		}

		if err = couchdb.UpdateDoc(instance, toPatch); err != nil {
			return err
		}

		return jsonapi.Data(c, http.StatusOK, &APIPermission{toPatch}, nil)
	}
}

func revokePermission(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	current, err := middlewares.GetPermission(c)
	if err != nil {
		return err
	}

	toRevoke, err := permission.GetByID(instance, c.Param("permdocid"))
	if err != nil {
		return err
	}

	// Check if the permission is linked to an OAuth Client
	if current.Client != nil {
		oauthClient := current.Client.(*oauth.Client)

		if auth.IsLinkedApp(oauthClient.SoftwareID) {
			slug := auth.GetLinkedAppSlug(oauthClient.SoftwareID)

			// Changing the sourceID from the OAuth clientID to the classic
			// io.cozy.apps/slug one
			current.SourceID = consts.Apps + "/" + slug

		}
	}

	if !current.ParentOf(toRevoke) {
		return permission.ErrNotParent
	}

	err = toRevoke.Revoke(instance)
	if err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// Routes sets the routing for the permissions service
func Routes(router *echo.Group) {
	// API Routes
	router.POST("", createPermission)
	router.GET("/self", displayPermissions)
	router.POST("/exists", listPermissions)
	router.PATCH("/:permdocid", patchPermission(permission.GetByID, "permdocid"))
	router.DELETE("/:permdocid", revokePermission)

	router.PATCH("/apps/:slug", patchPermission(permission.GetForWebapp, "slug"))
	router.PATCH("/konnectors/:slug", patchPermission(permission.GetForKonnector, "slug"))

	router.GET("/doctype/:doctype/shared-by-link", listByLinkPermissionsByDoctype)

	// Legacy routes, kept here for compatibility reasons
	router.GET("/doctype/:doctype/sharedByLink", listByLinkPermissionsByDoctype)
}
