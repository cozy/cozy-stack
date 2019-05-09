package instances

import (
	"net/http"
	"strconv"
	"time"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/echo"
)

func createToken(c echo.Context) error {
	domain := c.QueryParam("Domain")
	audience := c.QueryParam("Audience")
	scope := c.QueryParam("Scope")
	subject := c.QueryParam("Subject")
	in, err := lifecycle.GetInstance(domain)
	if err != nil {
		// With a cluster of couchdb, we can have a race condition where we
		// query an index before it has been updated for an instance that has
		// just been created.
		// Cf https://issues.apache.org/jira/browse/COUCHDB-3336
		time.Sleep(1 * time.Second)
		in, err = lifecycle.GetInstance(domain)
		if err != nil {
			return wrapError(err)
		}
	}
	issuedAt := time.Now()
	validity := consts.DefaultValidityDuration
	switch audience {
	case consts.AppAudience, "webapp":
		audience = consts.AppAudience
		validity = consts.AppTokenValidityDuration
	case consts.KonnectorAudience, "konnector":
		audience = consts.KonnectorAudience
		validity = consts.KonnectorTokenValidityDuration
	case consts.AccessTokenAudience, "access-token":
		audience = consts.AccessTokenAudience
		validity = consts.AccessTokenValidityDuration
	case consts.RefreshTokenAudience, "refresh-token":
		audience = consts.RefreshTokenAudience
	case consts.CLIAudience:
		audience = consts.CLIAudience
		validity = consts.CLITokenValidityDuration
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "Unknown audience %s", audience)
	}
	if e := c.QueryParam("Expire"); e != "" && e != "0s" {
		var d time.Duration
		if d, err = time.ParseDuration(e); err == nil {
			issuedAt = issuedAt.Add(d - validity)
		}
	}
	token, err := in.MakeJWT(audience, subject, scope, "", issuedAt)
	if err != nil {
		return err
	}
	return c.String(http.StatusOK, token)
}

func registerClient(c echo.Context) error {
	in, err := lifecycle.GetInstance(c.QueryParam("Domain"))
	if err != nil {
		return wrapError(err)
	}
	allowLoginScope, err := strconv.ParseBool(c.QueryParam("AllowLoginScope"))
	if err != nil {
		return wrapError(err)
	}

	client := oauth.Client{
		RedirectURIs:          []string{c.QueryParam("RedirectURI")},
		ClientName:            c.QueryParam("ClientName"),
		SoftwareID:            c.QueryParam("SoftwareID"),
		AllowLoginScope:       allowLoginScope,
		OnboardingSecret:      c.QueryParam("OnboardingSecret"),
		OnboardingApp:         c.QueryParam("OnboardingApp"),
		OnboardingPermissions: c.QueryParam("OnboardingPermissions"),
		OnboardingState:       c.QueryParam("OnboardingState"),
	}
	if regErr := client.Create(in); regErr != nil {
		return c.String(http.StatusBadRequest, regErr.Description)
	}
	return c.JSON(http.StatusOK, client)
}

func findClientBySoftwareID(c echo.Context) error {
	domain := c.QueryParam("domain")
	softwareID := c.QueryParam("software_id")

	inst, err := lifecycle.GetInstance(domain)
	if err != nil {
		return err
	}
	client, err := oauth.FindClientBySoftwareID(inst, softwareID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, client)
}
