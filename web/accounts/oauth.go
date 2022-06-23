package accounts

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/session"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/oidc"
	jwt "github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
)

type apiAccount struct {
	*account.Account
}

func (a *apiAccount) MarshalJSON() ([]byte, error)           { return json.Marshal(a.Account) }
func (a *apiAccount) Relationships() jsonapi.RelationshipMap { return nil }
func (a *apiAccount) Included() []jsonapi.Object             { return nil }
func (a *apiAccount) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/data/" + consts.Accounts + "/" + a.ID()}
}

func start(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	accountTypeID := c.Param("accountType")
	accountType, err := account.TypeInfo(accountTypeID, instance.ContextName)
	if err != nil {
		return err
	}

	state, err := getStorage().Add(&stateHolder{
		InstanceDomain: instance.Domain,
		AccountType:    accountType.ServiceID(),
		ClientState:    c.QueryParam("state"),
		Nonce:          c.QueryParam("nonce"),
		Slug:           c.QueryParam("slug"),
	})
	if err != nil {
		return err
	}

	url, err := accountType.MakeOauthStartURL(instance, state, c.QueryParams())
	if err != nil {
		return err
	}

	return c.Redirect(http.StatusSeeOther, url)
}

func redirectToApp(
	c echo.Context,
	inst *instance.Instance,
	acc *account.Account,
	clientState, slug, errorMessage string,
) error {
	if slug == "" {
		slug = consts.HomeSlug
	}
	u := inst.SubDomain(slug)
	vv := &url.Values{}
	if acc != nil {
		vv.Add("account", acc.ID())
	}
	if clientState != "" {
		vv.Add("state", clientState)
	}
	if errorMessage != "" {
		vv.Add("error", errorMessage)
	}
	u.RawQuery = vv.Encode()
	return c.Redirect(http.StatusSeeOther, u.String())
}

// redirect is the redirect_uri endpoint passed to oauth services
// it should create the account.
// middlewares.NeedInstance is not applied before this handler
// it needs to handle both
// - with instance redirect
// - without instance redirect
func redirect(c echo.Context) error {
	accessCode := c.QueryParam("code")
	accessToken := c.QueryParam("access_token")
	accountTypeID := c.Param("accountType")

	i, _ := lifecycle.GetInstance(c.Request().Host)
	var clientState, slug string
	var acc *account.Account

	if accessToken != "" {
		if i == nil {
			return echo.NewHTTPError(http.StatusBadRequest,
				"using ?access_token with instance-less redirect")
		}

		acc = &account.Account{
			AccountType: accountTypeID,
			Oauth: &account.OauthInfo{
				AccessToken: accessToken,
			},
		}
	} else {
		stateCode := c.QueryParam("state")
		state := getStorage().Find(stateCode)
		if state == nil ||
			state.AccountType != accountTypeID ||
			(i != nil && state.InstanceDomain != i.Domain) {
			return errors.New("bad state")
		}
		if i == nil {
			var err error
			i, err = lifecycle.GetInstance(state.InstanceDomain)
			if err != nil {
				return errors.New("bad state")
			}
		}

		clientState = state.ClientState
		slug = state.Slug

		// https://developers.google.com/identity/protocols/oauth2/web-server?hl=en#handlingresponse
		if c.QueryParam("error") == "access_denied" {
			return redirectToApp(c, i, nil, clientState, slug, "access_denied")
		}

		accountType, err := account.TypeInfo(accountTypeID, i.ContextName)
		if err != nil {
			return err
		}

		if state.ReconnectFlow {
			return redirectToApp(c, i, nil, clientState, slug, "")
		}

		if accountType.TokenEndpoint == "" {
			params := c.QueryParams()
			params.Del("state")
			acc = &account.Account{
				AccountType: accountTypeID,
				Oauth: &account.OauthInfo{
					ClientID:     accountType.ClientID,
					ClientSecret: accountType.ClientSecret,
					Query:        &params,
				},
			}
		} else {
			acc, err = accountType.RequestAccessToken(i, accessCode, stateCode, state.Nonce)
			if err != nil {
				return err
			}
		}
	}

	if err := couchdb.CreateDoc(i, acc); err != nil {
		return err
	}

	c.Set("instance", i.WithContextualDomain(c.Request().Host))
	return redirectToApp(c, i, acc, clientState, slug, "")
}

// refresh is an internal route used by konnectors to refresh accounts
// it requires permissions GET:io.cozy.accounts:accountid
func refresh(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	accountid := c.Param("accountid")

	var acc account.Account
	if err := couchdb.GetDoc(instance, consts.Accounts, accountid, &acc); err != nil {
		return err
	}

	if err := middlewares.Allow(c, permission.GET, &acc); err != nil {
		return err
	}

	accountType, err := account.TypeInfo(acc.AccountType, instance.ContextName)
	if err != nil {
		return err
	}

	err = accountType.RefreshAccount(acc)
	if err != nil {
		return err
	}

	err = couchdb.UpdateDoc(instance, &acc)
	if err != nil {
		return err
	}

	return jsonapi.Data(c, http.StatusOK, &apiAccount{&acc}, nil)
}

// reconnect can used to reconnect a user from BI
func reconnect(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	accountid := c.Param("accountid")

	var acc account.Account
	if err := couchdb.GetDoc(instance, consts.Accounts, accountid, &acc); err != nil {
		return err
	}

	accountType, err := account.TypeInfo(acc.AccountType, instance.ContextName)
	if err != nil {
		return err
	}

	state, err := getStorage().Add(&stateHolder{
		InstanceDomain: instance.Domain,
		AccountType:    accountType.ServiceID(),
		ClientState:    c.QueryParam("state"),
		Slug:           c.QueryParam("slug"),
		ReconnectFlow:  true,
	})
	if err != nil {
		return err
	}

	url, err := accountType.MakeReconnectURL(instance, state, c.QueryParams())
	if err != nil {
		return err
	}

	return c.Redirect(http.StatusSeeOther, url)
}

func checkLogin(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		inst := middlewares.GetInstance(c)
		isLoggedIn := middlewares.IsLoggedIn(c)
		wasLoggedIn := isLoggedIn

		if code := c.QueryParam("session_code"); code != "" {
			// XXX we should always clear the session code to avoid it being
			// reused, even if the user is already logged in and we don't want to
			// create a new session
			if checked := inst.CheckAndClearSessionCode(code); checked {
				isLoggedIn = true
			}
		}

		if !isLoggedIn && checkIDToken(c) {
			isLoggedIn = true
		}

		if !isLoggedIn {
			return echo.NewHTTPError(http.StatusForbidden)
		}

		if !wasLoggedIn {
			sessionID, err := auth.SetCookieForNewSession(c, session.ShortRun)
			req := c.Request()
			if err == nil {
				if err = session.StoreNewLoginEntry(inst, sessionID, "", req, "session_code", false); err != nil {
					inst.Logger().Errorf("Could not store session history %q: %s", sessionID, err)
				}
			}
		}

		return next(c)
	}
}

func checkIDToken(c echo.Context) bool {
	inst := middlewares.GetInstance(c)
	cfg, ok := config.GetOIDC(inst.ContextName)
	if !ok {
		return false
	}
	allowOAuthToken, _ := cfg["allow_oauth_token"].(bool)
	if !allowOAuthToken {
		return false
	}
	idTokenKeyURL, _ := cfg["id_token_jwk_url"].(string)
	if idTokenKeyURL == "" {
		return false
	}

	keys, err := oidc.GetIDTokenKeys(idTokenKeyURL)
	if err != nil {
		return false
	}
	idToken := c.QueryParam("id_token")
	token, err := jwt.Parse(idToken, func(token *jwt.Token) (interface{}, error) {
		return oidc.ChooseKeyForIDToken(keys, token)
	})
	if err != nil {
		logger.WithNamespace("oidc").Errorf("Error on jwt.Parse: %s", err)
		return false
	}
	if !token.Valid {
		logger.WithNamespace("oidc").Errorf("Invalid token: %#v", token)
		return false
	}
	claims := token.Claims.(jwt.MapClaims)
	if claims["sub"] == "" || claims["sub"] != inst.OIDCID {
		inst.Logger().WithNamespace("oidc").Errorf("Invalid sub: %s != %s", claims["sub"], inst.OIDCID)
		return false
	}

	return true
}

// Routes setups routing for cozy-as-oauth-client routes
// Careful, the normal middlewares NeedInstance and LoadSession are not applied
// to this group in web/routing
func Routes(router *echo.Group) {
	router.GET("/:accountType/start", start, middlewares.NeedInstance, middlewares.LoadSession, checkLogin)
	router.GET("/:accountType/redirect", redirect)
	router.POST("/:accountType/:accountid/refresh", refresh, middlewares.NeedInstance)
	router.GET("/:accountType/:accountid/reconnect", reconnect, middlewares.NeedInstance, middlewares.LoadSession, checkLogin)
}
