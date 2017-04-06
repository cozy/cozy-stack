package permissions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

// exports all constants from pkg/permissions to avoid double imports
var (
	ALL    = permissions.ALL
	GET    = permissions.GET
	PUT    = permissions.PUT
	POST   = permissions.POST
	PATCH  = permissions.PATCH
	DELETE = permissions.DELETE
)

// ErrPatchCodeOrSet is returned when an attempt is made to patch both
// code & set in one request
var ErrPatchCodeOrSet = echo.NewHTTPError(http.StatusBadRequest,
	"The patch doc should have property 'codes' or 'permissions', not both")

// ErrForbidden is returned when a bad operation is attempted on permissions
var ErrForbidden = echo.NewHTTPError(http.StatusForbidden)

// ContextPermissionSet is the key used in echo context to store permissions set
const ContextPermissionSet = "permissions_set"

// ContextClaims is the key used in echo context to store claims
// #nosec
const ContextClaims = "token_claims"

type apiPermission struct {
	*permissions.Permission
}

func (p *apiPermission) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.Permission)
}

// Relationships implements jsonapi.Doc
func (p *apiPermission) Relationships() jsonapi.RelationshipMap { return nil }

// Included implements jsonapi.Doc
func (p *apiPermission) Included() []jsonapi.Object { return nil }

// Links implements jsonapi.Doc
func (p *apiPermission) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/permissions/" + p.PID}
}

func displayPermissions(c echo.Context) error {
	doc, err := GetPermission(c)

	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, doc.Permissions)
}

func createPermission(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	names := strings.Split(c.QueryParam("codes"), ",")
	parent, err := GetPermission(c)
	if err != nil {
		return err
	}

	var subdoc permissions.Permission
	if _, err = jsonapi.Bind(c.Request(), &subdoc); err != nil {
		return err
	}

	var codes map[string]string
	if names != nil {
		codes = make(map[string]string, len(names))
		for _, name := range names {
			codes[name], err = crypto.NewJWT(instance.OAuthSecret, &permissions.Claims{
				StandardClaims: jwt.StandardClaims{
					Audience: permissions.ShareAudience,
					Issuer:   instance.Domain,
					IssuedAt: crypto.Timestamp(),
					Subject:  name,
				},
				Scope: "",
			})
			if err != nil {
				return err
			}
		}
	}

	if parent == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "no parent")
	}

	pdoc, err := permissions.CreateShareSet(instance, parent, codes, subdoc.Permissions)
	if err != nil {
		return err
	}

	return jsonapi.Data(c, http.StatusOK, &apiPermission{pdoc}, nil)
}

type refAndVerb struct {
	ID      string               `json:"id"`
	DocType string               `json:"type"`
	Verbs   *permissions.VerbSet `json:"verbs"`
}

const limitPermissionsByDoctype = 30

func listPermissionsByDoctype(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	doctype := c.Param("doctype")
	current, err := GetPermission(c)
	if err != nil {
		return err
	}

	if !current.Permissions.AllowWholeType("GET", doctype) {
		return jsonapi.NewError(http.StatusForbidden,
			"you need GET permission on whole type to list its permissions")
	}

	cursor, err := jsonapi.ExtractPaginationCursor(c, limitPermissionsByDoctype)
	if err != nil {
		return err
	}

	perms, err := permissions.GetPermissionsByType(instance, doctype, cursor)
	if err != nil {
		return err
	}

	links := &jsonapi.LinksList{}
	if !cursor.Done {
		params, err := jsonapi.PaginationCursorToParams(cursor)
		if err != nil {
			return err
		}
		links.Next = fmt.Sprintf("/permissions/doctype/%s?%s", doctype, params.Encode())
	}

	out := make([]jsonapi.Object, len(perms))
	for i, p := range perms {
		out[i] = &apiPermission{p}
	}

	return jsonapi.DataList(c, http.StatusOK, out, links)
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
		result, err2 := permissions.GetPermissionsForIDs(instance, doctype, idSlice)
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

func patchPermission(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	current, err := GetPermission(c)
	if err != nil {
		return err
	}

	var patch permissions.Permission
	if _, err = jsonapi.Bind(c.Request(), &patch); err != nil {
		return err
	}

	patchSet := patch.Permissions != nil && len(patch.Permissions) > 0
	patchCodes := len(patch.Codes) > 0

	if patchCodes == patchSet {
		return ErrPatchCodeOrSet
	}

	toPatch, err := permissions.GetByID(instance, c.Param("permdocid"))
	if err != nil {
		return err
	}

	if patchCodes {
		// a permission can be updated only by its parent
		if !current.ParentOf(toPatch) {
			return ErrForbidden
		}
		toPatch.PatchCodes(patch.Codes)
	}

	if patchSet {
		// I can only add my own permissions to another permission doc
		if !patch.Permissions.IsSubSetOf(current.Permissions) {
			return ErrForbidden
		}
		toPatch.AddRules(patch.Permissions...)
	}

	if err = couchdb.UpdateDoc(instance, toPatch); err != nil {
		return err
	}

	return jsonapi.Data(c, http.StatusOK, &apiPermission{toPatch}, nil)
}

func revokePermission(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	current, err := GetPermission(c)
	if err != nil {
		return err
	}

	toRevoke, err := permissions.GetByID(instance, c.Param("permdocid"))
	if err != nil {
		return err
	}

	// a permission can be revoked only by its parent
	if !current.ParentOf(toRevoke) {
		return ErrForbidden
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
	router.GET("/doctype/:doctype", listPermissionsByDoctype)
	router.GET("/self", displayPermissions)
	router.POST("/exists", listPermissions)
	router.PATCH("/:permdocid", patchPermission)
	router.DELETE("/:permdocid", revokePermission)
}
