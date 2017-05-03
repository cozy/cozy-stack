package oauth

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/pkg/accounts"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

type apiAccount struct {
	*accounts.Account
}

func (a *apiAccount) MarshalJSON() ([]byte, error)           { return json.Marshal(a.Account) }
func (a *apiAccount) Relationships() jsonapi.RelationshipMap { return nil }
func (a *apiAccount) Included() []jsonapi.Object             { return nil }
func (a *apiAccount) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/data/" + consts.Accounts + "/" + a.ID()}
}

func start(c echo.Context) error {
	scope := c.QueryParam("scope")
	clientState := c.QueryParam("state")
	accountTypeID := c.Param("accountType")
	accountType, err := accounts.TypeInfo(accountTypeID)
	if err != nil {
		return err
	}

	url, err := accountType.MakeOauthStartURL(scope, clientState)
	if err != nil {
		return err
	}

	return c.Redirect(http.StatusSeeOther, url)
}

// redirect is the redirect_uri endpoint passed to oauth services
// it should create the account.
func redirect(c echo.Context) error {

	instance := middlewares.GetInstance(c)

	accessCode := c.QueryParam("code")
	accountTypeID := c.Param("accountType")
	accountType, err := accounts.TypeInfo(accountTypeID)
	if err != nil {
		return err
	}

	account, err := accountType.AccessCodeToAccessToken(accessCode)
	if err != nil {
		return err
	}

	err = couchdb.CreateDoc(instance, account)
	if err != nil {
		return err
	}

	u := instance.SubDomain(consts.DataConnectSlug)
	vv := &url.Values{}
	vv.Add("account", account.ID())
	vv.Add("state", c.QueryParam("state"))
	u.RawQuery = vv.Encode()
	return c.Redirect(http.StatusSeeOther, u.String())
}

// refresh is an internal route used by konnectors to refresh accounts
// it requires permissions GET:io.cozy.accounts:accountid
func refresh(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	accountid := c.Param("accountid")

	var account accounts.Account
	if err := couchdb.GetDoc(instance, consts.Accounts, accountid, &account); err != nil {
		return err
	}

	accountType, err := accounts.TypeInfo(account.AccountType)
	if err != nil {
		return err
	}

	err = accountType.RefreshAccount(account)
	if err != nil {
		return err
	}

	err = couchdb.UpdateDoc(instance, &account)
	if err != nil {
		return err
	}

	return jsonapi.Data(c, http.StatusOK, &apiAccount{&account}, nil)

}

// Routes setups routing for cozy-as-oauth-client routes
func Routes(router *echo.Group) {
	router.GET("/start/:accountType", start)
	router.GET("/redirect/:accountType", redirect)
	router.GET("/refresh/:accountid", refresh)
}
