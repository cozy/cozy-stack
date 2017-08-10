package instances

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/labstack/echo"
)

type apiInstance struct {
	*instance.Instance
}

func (i *apiInstance) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.Instance)
}

// Links is used to generate a JSON-API link for the instance
func (i *apiInstance) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/instances/" + i.Instance.DocID}
}

// Relationships is used to generate the content relationship in JSON-API format
func (i *apiInstance) Relationships() jsonapi.RelationshipMap {
	return jsonapi.RelationshipMap{}
}

// Included is part of the jsonapi.Object interface
func (i *apiInstance) Included() []jsonapi.Object {
	return nil
}

func createHandler(c echo.Context) error {
	var diskQuota int64
	if c.QueryParam("DiskQuota") != "" {
		var err error
		diskQuota, err = strconv.ParseInt(c.QueryParam("DiskQuota"), 10, 64)
		if err != nil {
			return wrapError(err)
		}
	}
	var settings couchdb.JSONDoc
	settings.M = make(map[string]interface{})
	for _, setting := range strings.Split(c.QueryParam("Settings"), ",") {
		if parts := strings.SplitN(setting, ":", 2); len(parts) == 2 {
			settings.M[parts[0]] = parts[1]
		}
	}
	if tz := c.QueryParam("Timezone"); tz != "" {
		settings.M["tz"] = tz
	}
	if email := c.QueryParam("Email"); email != "" {
		settings.M["email"] = email
	}
	if name := c.QueryParam("PublicName"); name != "" {
		settings.M["public_name"] = name
	}
	in, err := instance.Create(&instance.Options{
		Domain:    c.QueryParam("Domain"),
		Locale:    c.QueryParam("Locale"),
		DiskQuota: diskQuota,
		Settings:  settings,
		Apps:      utils.SplitTrimString(c.QueryParam("Apps"), ","),
		Dev:       (c.QueryParam("Dev") == "true"),
	})
	if err != nil {
		return wrapError(err)
	}
	in.OAuthSecret = nil
	in.SessionSecret = nil
	in.PassphraseHash = nil
	pass := c.QueryParam("Passphrase")
	if pass != "" {
		if err = in.RegisterPassphrase([]byte(pass), in.RegisterToken); err != nil {
			return err
		}
	}
	return jsonapi.Data(c, http.StatusCreated, &apiInstance{in}, nil)
}

func showHandler(c echo.Context) error {
	domain := c.Param("domain")
	i, err := instance.Get(domain)
	if err != nil {
		return wrapError(err)
	}
	return jsonapi.Data(c, http.StatusOK, &apiInstance{i}, nil)
}

func modifyHandler(c echo.Context) error {
	domain := c.Param("domain")
	i, err := instance.Get(domain)
	if err != nil {
		return wrapError(err)
	}
	var shouldUpdate bool
	if quota := c.QueryParam("DiskQuota"); quota != "" {
		var diskQuota int64
		diskQuota, err = strconv.ParseInt(quota, 10, 64)
		if err != nil {
			return wrapError(err)
		}
		i.BytesDiskQuota = diskQuota
		shouldUpdate = true
	}
	if locale := c.QueryParam("Locale"); locale != "" {
		i.Locale = locale
		shouldUpdate = true
	}
	if shouldUpdate {
		if err = instance.Update(i); err != nil {
			return wrapError(err)
		}
	}
	if debug, err := strconv.ParseBool(c.QueryParam("Debug")); err == nil {
		if debug {
			err = logger.AddDebugDomain(domain)
		} else {
			err = logger.RemoveDebugDomain(domain)
		}
		if err != nil {
			return wrapError(err)
		}
	}
	return jsonapi.Data(c, http.StatusOK, &apiInstance{i}, nil)
}

func listHandler(c echo.Context) error {
	is, err := instance.List()
	if err != nil {
		if couchdb.IsNoDatabaseError(err) {
			return jsonapi.DataList(c, http.StatusOK, nil, nil)
		}
		return wrapError(err)
	}

	objs := make([]jsonapi.Object, len(is))
	for i, in := range is {
		in.OAuthSecret = nil
		in.SessionSecret = nil
		in.PassphraseHash = nil
		objs[i] = &apiInstance{in}
	}

	return jsonapi.DataList(c, http.StatusOK, objs, nil)
}

func deleteHandler(c echo.Context) error {
	domain := c.Param("domain")
	err := instance.Destroy(domain)
	if err != nil {
		return wrapError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func createToken(c echo.Context) error {
	domain := c.QueryParam("Domain")
	audience := c.QueryParam("Audience")
	scope := c.QueryParam("Scope")
	subject := c.QueryParam("Subject")
	expire := c.QueryParam("Expire")
	in, err := instance.Get(domain)
	if err != nil {
		return wrapError(err)
	}
	switch audience {
	case "app":
		audience = permissions.AppAudience
	case "konn", "konnector":
		audience = permissions.KonnectorAudience
	case "access-token":
		audience = permissions.AccessTokenAudience
	case "cli":
		audience = permissions.CLIAudience
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "Unknown audience %s", audience)
	}
	issuedAt := time.Now()
	if expire != "" && expire != "0s" {
		var duration time.Duration
		if duration, err = time.ParseDuration(expire); err == nil {
			issuedAt = issuedAt.Add(duration - permissions.TokenValidityDuration)
		}
	}
	token, err := in.MakeJWT(audience, subject, scope, issuedAt)
	if err != nil {
		return err
	}
	return c.String(http.StatusOK, token)
}

func registerClient(c echo.Context) error {
	in, err := instance.Get(c.QueryParam("Domain"))
	if err != nil {
		return wrapError(err)
	}
	client := oauth.Client{
		RedirectURIs: []string{c.QueryParam("RedirectURI")},
		ClientName:   c.QueryParam("ClientName"),
		SoftwareID:   c.QueryParam("SoftwareID"),
	}
	if regErr := client.Create(in); regErr != nil {
		return c.String(http.StatusBadRequest, regErr.Description)
	}
	return c.String(http.StatusOK, client.ClientID)
}

func wrapError(err error) error {
	switch err {
	case instance.ErrNotFound:
		return jsonapi.NotFound(err)
	case instance.ErrExists:
		return jsonapi.Conflict(err)
	case instance.ErrIllegalDomain:
		return jsonapi.InvalidParameter("domain", err)
	case instance.ErrMissingToken:
		return jsonapi.BadRequest(err)
	case instance.ErrInvalidToken:
		return jsonapi.BadRequest(err)
	case instance.ErrMissingPassphrase:
		return jsonapi.BadRequest(err)
	case instance.ErrInvalidPassphrase:
		return jsonapi.BadRequest(err)
	}
	return err
}

// Routes sets the routing for the instances service
func Routes(router *echo.Group) {
	router.GET("", listHandler)
	router.POST("", createHandler)
	router.GET("/:domain", showHandler)
	router.PATCH("/:domain", modifyHandler)
	router.DELETE("/:domain", deleteHandler)
	router.POST("/token", createToken)
	router.POST("/oauth_client", registerClient)
}
