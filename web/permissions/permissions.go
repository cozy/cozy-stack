package permissions

import (
	"encoding/json"
	"net/http"
	"strings"

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

// ContextPermissionSet is the key used in echo context to store permissions set
const ContextPermissionSet = "permissions_set"

// ContextClaims is the key used in echo context to store claims
// #nosec
const ContextClaims = "token_claims"

func displayPermissions(c echo.Context) error {
	doc, err := getPermission(c)

	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, doc.Permissions)
}

func createPermission(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	names := strings.Split(c.QueryParam("codes"), ",")
	parent, err := getPermission(c)
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

	return jsonapi.Data(c, http.StatusOK, pdoc, nil)
}

type refAndVerb struct {
	ID      string               `json:"id"`
	DocType string               `json:"type"`
	Verbs   *permissions.VerbSet `json:"verbs"`
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

// Routes sets the routing for the status service
func Routes(router *echo.Group) {
	// API Routes
	router.POST("", createPermission)
	router.GET("/self", displayPermissions)
	router.POST("/exists", listPermissions)
}
