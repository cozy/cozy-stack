package accounts

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
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
	if !middlewares.IsLoggedIn(c) {
		return echo.NewHTTPError(http.StatusForbidden)
	}

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

func redirectToApp(c echo.Context, acc *account.Account, clientState, slug string) error {
	instance := middlewares.GetInstance(c)
	if slug == "" {
		slug = consts.HomeSlug
	}
	u := instance.SubDomain(slug)
	vv := &url.Values{}
	if acc != nil {
		vv.Add("account", acc.ID())
	}
	if clientState != "" {
		vv.Add("state", clientState)
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

		// https://developers.google.com/identity/protocols/oauth2/web-server?hl=en#handlingresponse
		if c.QueryParam("error") == "access_denied" {
			u := i.SubDomain(consts.StoreSlug)
			u.Fragment = "/discover/" + accountTypeID
			return c.Redirect(http.StatusSeeOther, u.String())
		}

		accountType, err := account.TypeInfo(accountTypeID, i.ContextName)
		if err != nil {
			return err
		}

		clientState = state.ClientState
		slug = state.Slug

		if state.ReconnectFlow {
			return redirectToApp(c, nil, clientState, slug)
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
	return redirectToApp(c, acc, clientState, slug)
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

	if err := middlewares.Allow(c, permission.GET, &acc); err != nil {
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

// Routes setups routing for cozy-as-oauth-client routes
// Careful, the normal middlewares NeedInstance and LoadSession are not applied
// to this group in web/routing
func Routes(router *echo.Group) {
	router.GET("/:accountType/start", start, middlewares.NeedInstance, middlewares.LoadSession)
	router.GET("/:accountType/redirect", redirect)
	router.POST("/:accountType/:accountid/refresh", refresh, middlewares.NeedInstance)
	router.GET("/:accountType/:accountid/reconnect", reconnect, middlewares.NeedInstance)
}
