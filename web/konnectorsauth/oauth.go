package konnectorsauth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/accounts"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	webperm "github.com/cozy/cozy-stack/web/permissions"
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
	instance := middlewares.GetInstance(c)

	scope := c.QueryParam("scope")
	clientState := c.QueryParam("state")
	nonce := c.QueryParam("nonce")
	accountTypeID := c.Param("accountType")
	accountType, err := accounts.TypeInfo(accountTypeID)
	if err != nil {
		return err
	}

	state, err := getStorage().Add(&stateHolder{
		InstanceDomain: instance.Domain,
		AccountType:    accountType.ID(),
		ClientState:    clientState,
		Nonce:          nonce,
	})
	if err != nil {
		return err
	}

	url, err := accountType.MakeOauthStartURL(instance, scope, state)
	if err != nil {
		return err
	}

	return c.Redirect(http.StatusSeeOther, url)
}

func redirectToDataCollect(c echo.Context, account *accounts.Account, clientState string) error {
	instance := middlewares.GetInstance(c)
	u := instance.SubDomain(consts.CollectSlug)
	vv := &url.Values{}
	vv.Add("account", account.ID())
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
	accountType, err := accounts.TypeInfo(accountTypeID)
	if err != nil {
		return err
	}

	i, _ := instance.Get(c.Request().Host)

	if i != nil && accessToken != "" {
		account := &accounts.Account{
			AccountType: accountTypeID,
			Oauth: &accounts.OauthInfo{
				AccessToken: accessToken,
			},
		}
		err = couchdb.CreateDoc(i, account)
		if err != nil {
			return err
		}
		c.Set("instance", i)
		return redirectToDataCollect(c, account, "")
	}

	if accessToken != "" {
		return echo.NewHTTPError(http.StatusBadRequest,
			"using ?access_token with instance-less redirect")
	}

	stateCode := c.QueryParam("state")
	state := getStorage().Find(stateCode)

	if state == nil ||
		state.AccountType != accountTypeID ||
		(i != nil && state.InstanceDomain != i.Domain) {
		return errors.New("bad state")
	}

	// TODO should we check if the req.Host is a given "_oauth_callback" domain?
	if i == nil {
		i, err = instance.Get(state.InstanceDomain)
		if err != nil {
			return errors.New("bad state")
		}
	}

	var req *http.Request

	data := url.Values{
		"grant_type":   []string{accounts.AuthorizationCode},
		"code":         []string{accessCode},
		"redirect_uri": []string{accountType.RedirectURI(i)},
		"state":        []string{stateCode},
		"nonce":        []string{state.Nonce},
	}

	if accountType.TokenAuthMode != accounts.BasicTokenAuthMode {
		data.Add("client_id", accountType.ClientID)
		data.Add("client_secret", accountType.ClientSecret)
	}

	body := data.Encode()
	req, err = http.NewRequest("POST", accountType.TokenEndpoint, strings.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Accept", "application/json")

	if accountType.TokenAuthMode == accounts.BasicTokenAuthMode {
		auth := []byte(accountType.ClientID + ":" + accountType.ClientSecret)
		req.Header.Add("Authorization", "Basic "+base64.StdEncoding.EncodeToString(auth))
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	resBody, err := ioutil.ReadAll(res.Body)

	if res.StatusCode != 200 {
		return errors.New("oauth services responded with non-200 res : " + string(resBody))
	}

	if err != nil {
		return err
	}

	var out struct {
		RefreshToken     string `json:"refresh_token"`
		AccessToken      string `json:"access_token"`
		IDToken          string `json:"id_token"` // alternative name for access_token
		ExpiresIn        int    `json:"expires_in"`
		TokenType        string `json:"token_type"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	err = json.Unmarshal(resBody, &out)
	if err != nil {
		return err
	}

	if out.Error != "" {
		return fmt.Errorf("OauthError(%s) %s", out.Error, out.ErrorDescription)
	}

	var ExpiresAt time.Time
	if out.ExpiresIn != 0 {
		ExpiresAt = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	}

	account := &accounts.Account{
		AccountType: accountType.ID(),
		Oauth:       &accounts.OauthInfo{ExpiresAt: ExpiresAt},
	}

	if out.AccessToken == "" {
		out.AccessToken = out.IDToken
	}

	if out.AccessToken == "" {
		return errors.New("server responded without access token")
	}

	account.Oauth.AccessToken = out.AccessToken
	account.Oauth.RefreshToken = out.RefreshToken
	account.Oauth.TokenType = out.TokenType

	// decode same resBody into a map for non-standard fields
	var extras map[string]interface{}
	json.Unmarshal(resBody, &extras)
	delete(extras, "access_token")
	delete(extras, "refresh_token")
	delete(extras, "token_type")
	delete(extras, "expires_in")

	if len(extras) > 0 {
		account.Extras = extras
	}

	err = couchdb.CreateDoc(i, account)
	if err != nil {
		return err
	}

	c.Set("instance", i)
	return redirectToDataCollect(c, account, state.ClientState)
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

	if err := webperm.Allow(c, permissions.GET, &account); err != nil {
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
// Careful, the normal middlewares NeedInstance and LoadSession are not applied
// to this group in web/routing
func Routes(router *echo.Group) {
	router.GET("/:accountType/start", start, middlewares.NeedInstance)
	router.GET("/:accountType/redirect", redirect)
	router.POST("/:accountType/:accountid/refresh", refresh, middlewares.NeedInstance)
}
