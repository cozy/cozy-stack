package auth

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/oauth"
	oidcbinding "github.com/cozy/cozy-stack/model/oidc/binding"
	oidcprovider "github.com/cozy/cozy-stack/model/oidc/provider"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/limits"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

const (
	tokenExchangeAdminRole             = "admin"
	tokenExchangeOwnerRole             = "owner"
	tokenExchangeAllowedScope          = "io.cozy.files"
	tokenExchangeOAuthClientName       = "Twake Admin Panel"
	tokenExchangeOAuthClientSoftwareID = "twake-admin-panel"
)

type tokenExchangeRequest struct {
	IDToken string `json:"id_token"`
	Scope   string `json:"scope"`
}

type tokenExchangeResponse struct {
	AccessTokenReponse
	ClientID          string `json:"client_id"`
	ClientSecret      string `json:"client_secret"`
	RegistrationToken string `json:"registration_access_token"`
}

func executeTokenExchange(c echo.Context, inst *instance.Instance, req tokenExchangeRequest) (*tokenExchangeResponse, error) {
	claims, err := validateTokenExchangeIDToken(inst, req.IDToken)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Scope != tokenExchangeAllowedScope {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "invalid scope")
	}

	client, err := createTokenExchangeOAuthClient(c, inst)
	if err != nil {
		return nil, err
	}
	defer LockOAuthClient(inst, client.ClientID)()

	if err := bindTokenExchangeOIDCSession(inst, client, claims); err != nil {
		if delErr := client.Delete(inst); delErr != nil {
			inst.Logger().WithNamespace("token_exchange").Warnf("Cannot delete orphaned OAuth client %s: %s", client.CouchID, delErr.Description)
		}
		return nil, err
	}

	return buildTokenExchangeResponse(inst, client, req.Scope)
}

func validateTokenExchangeIDToken(inst *instance.Instance, raw string) (jwt.MapClaims, error) {
	if inst == nil {
		return nil, errors.New("instance is missing")
	}
	conf, err := oidcprovider.LoadConfig(
		inst.ContextName,
		oidcprovider.RequireClientID,
		oidcprovider.RequireIDTokenKeyURL,
		oidcprovider.RequireIssuerOrTokenURL,
	)
	if err != nil || !conf.AllowOAuthToken {
		return nil, errors.New("this endpoint is not enabled")
	}

	claims, err := oidcprovider.VerifyIDToken(raw, conf)
	if err != nil {
		return nil, errors.New("invalid token")
	}

	expectedIssuer, err := oidcprovider.GetIssuer(inst.ContextName, conf)
	if err != nil {
		return nil, errors.New("invalid token")
	}
	issuer, err := claims.GetIssuer()
	if err != nil || issuer == "" || issuer != expectedIssuer {
		return nil, errors.New("invalid token issuer")
	}
	if !tokenExchangeAudienceMatches(claims, conf.ClientID) {
		return nil, errors.New("invalid token audience")
	}
	issuedAt, err := claims.GetIssuedAt()
	if err != nil || issuedAt == nil {
		return nil, errors.New("invalid token")
	}
	if issuedAt.Time.After(time.Now().Add(5 * time.Minute)) {
		return nil, errors.New("invalid token")
	}
	if !tokenExchangeHasAdminRole(claims) {
		return nil, errors.New("admin authorization is required")
	}

	orgID, _ := tokenExchangeClaimString(claims, "org_id")
	if orgID == "" || subtle.ConstantTimeCompare([]byte(orgID), []byte(inst.OrgID)) == 0 {
		return nil, errors.New("org_id mismatch")
	}
	if inst.OrgDomain != "" {
		orgDomain, ok := tokenExchangeClaimString(claims, "org_domain")
		if !ok || orgDomain == "" {
			return nil, errors.New("org_domain claim is required")
		}
		if subtle.ConstantTimeCompare([]byte(orgDomain), []byte(inst.OrgDomain)) == 0 {
			return nil, errors.New("org_domain mismatch")
		}
	}

	return claims, nil
}

func createTokenExchangeOAuthClient(c echo.Context, inst *instance.Instance) (*oauth.Client, error) {
	redirectURI, err := tokenExchangeRedirectURI(c, inst)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := config.GetRateLimiter().CheckRateLimit(inst, limits.OAuthClientType); limits.IsLimitReachedOrExceeded(err) {
		return nil, echo.NewHTTPError(http.StatusNotFound, "Not found")
	}

	client := &oauth.Client{
		RedirectURIs: []string{redirectURI},
		ClientName:   tokenExchangeOAuthClientName,
		ClientKind:   "browser",
		SoftwareID:   tokenExchangeOAuthClientSoftwareID,
	}
	if regErr := client.Create(inst); regErr != nil {
		return nil, echo.NewHTTPError(regErr.Code, regErr.Description)
	}

	storedClient, err := oauth.FindClient(inst, client.ClientID)
	if err != nil {
		return nil, err
	}
	storedClient.RegistrationToken = client.RegistrationToken
	return storedClient, nil
}

func bindTokenExchangeOIDCSession(inst *instance.Instance, client *oauth.Client, claims jwt.MapClaims) error {
	oldSessionID := client.OIDCSessionID
	sessionID, _ := tokenExchangeClaimString(claims, "sid")
	if sessionID != "" {
		client.OIDCSessionID = sessionID
	}
	if !client.Pending && sessionID == "" {
		return nil
	}

	client.Pending = false
	client.ClientID = ""
	if err := couchdb.UpdateDoc(inst, client); err != nil {
		inst.Logger().WithNamespace("oidc").Warnf("Cannot update OAuth client %s: %s", client.CouchID, err)
		return err
	}

	if sessionID != "" {
		if oldSessionID != "" && oldSessionID != sessionID {
			if err := oidcbinding.UnbindOAuthClient(inst.ContextName, inst.Domain, oldSessionID, client.CouchID); err != nil {
				inst.Logger().WithNamespace("oidc").Warnf("Cannot unbind OIDC session %s from OAuth client %s: %s", oldSessionID, client.CouchID, err)
			}
		}
		if err := oidcbinding.BindOAuthClient(inst.ContextName, inst.Domain, sessionID, client.CouchID); err != nil {
			inst.Logger().WithNamespace("oidc").Errorf("Cannot bind OIDC session %s to OAuth client %s: %s", sessionID, client.CouchID, err)
			return fmt.Errorf("cannot bind OIDC session to OAuth client: %w", err)
		}
	}

	client.ClientID = client.CouchID
	return nil
}

func buildTokenExchangeResponse(inst *instance.Instance, client *oauth.Client, scope string) (*tokenExchangeResponse, error) {
	out := &tokenExchangeResponse{
		AccessTokenReponse: AccessTokenReponse{
			Type:  "bearer",
			Scope: scope,
		},
		ClientID:          client.ClientID,
		ClientSecret:      client.ClientSecret,
		RegistrationToken: client.RegistrationToken,
	}

	refreshToken, err := client.CreateJWT(inst, consts.RefreshTokenAudience, scope)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "Can't generate refresh token")
	}
	out.Refresh = refreshToken

	accessToken, err := client.CreateJWT(inst, consts.AccessTokenAudience, scope)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "Can't generate access token")
	}
	out.Access = accessToken

	client.LastRefreshedAt = time.Now()
	if err := couchdb.UpdateDoc(inst, client); err != nil {
		inst.Logger().WithNamespace("token_exchange").Warnf("Cannot update LastRefreshedAt for client %s: %s", client.CouchID, err)
	}

	return out, nil
}

func tokenExchangeAudienceMatches(claims jwt.MapClaims, clientID string) bool {
	aud, err := claims.GetAudience()
	if err == nil && len(aud) > 0 {
		for _, value := range aud {
			if value == clientID {
				return true
			}
		}
		return false
	}
	azp, _ := tokenExchangeClaimString(claims, "azp")
	return azp == clientID
}

func tokenExchangeHasAdminRole(claims jwt.MapClaims) bool {
	orgRole, ok := tokenExchangeClaimString(claims, "org_role")
	return ok && (strings.EqualFold(orgRole, tokenExchangeAdminRole) ||
		strings.EqualFold(orgRole, tokenExchangeOwnerRole))
}

func tokenExchangeClaimString(claims jwt.MapClaims, key string) (string, bool) {
	raw, ok := claims[key]
	if !ok || raw == nil {
		return "", false
	}
	value, ok := raw.(string)
	return value, ok
}

func tokenExchangeRedirectURI(c echo.Context, inst *instance.Instance) (string, error) {
	if origin := c.Request().Header.Get(echo.HeaderOrigin); origin != "" {
		u, err := url.Parse(origin)
		if err == nil && u.Scheme != "" && u.Host != "" && utils.StripPort(u.Host) != inst.Domain {
			u.Path = ""
			u.RawQuery = ""
			u.Fragment = ""
			return u.String(), nil
		}
	}
	if inst != nil && inst.OrgDomain != "" {
		return "https://admin." + inst.OrgDomain, nil
	}
	return "", errors.New("cannot determine redirect URI")
}
