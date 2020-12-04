package move

import (
	"errors"
	"net/url"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

const (
	// MoveScope is the scope requested for a move (when we don't know yet if
	// the cozy will the source or the target).
	MoveScope = consts.ExportsRequests + " " + consts.Imports
	// SourceClientID is the fake OAuth client ID used for some move endpoints.
	SourceClientID = "move"
)

// Request is a struct for confirming a move to another Cozy.
type Request struct {
	SourceCreds RequestCredentials
	TargetCreds RequestCredentials
	Target      string
	Link        string `json:"-"`
}

// RequestCredentials is struct for OAuth credentials (access_token, client_id
// and client_secret).
type RequestCredentials struct {
	Token        string
	ClientID     string
	ClientSecret string
}

// TargetHost returns the host part of the target instance address.
func (r *Request) TargetHost() string {
	if u, err := url.Parse(r.Target); err == nil {
		return u.Host
	}
	return r.Target
}

// CreateRequestClient creates an OAuth client that can be used for move requests.
func CreateRequestClient(inst *instance.Instance) (*oauth.Client, error) {
	client := &oauth.Client{
		RedirectURIs: []string{config.GetConfig().Move.URL + "/fake"},
		ClientName:   "cozy-stack",
		SoftwareID:   "github.com/cozy/cozy-stack",
	}
	if err := client.Create(inst); err != nil {
		return nil, errors.New(err.Error)
	}
	return client, nil
}

// CreateRequest checks if the parameters are OK for moving, and if yes, it
// will persist them and return a link that can be used to confirm the move.
func CreateRequest(inst *instance.Instance, params url.Values) (*Request, error) {
	var source RequestCredentials
	code := params.Get("code")
	if code == "" {
		source.Token = params.Get("token")
		if source.Token == "" {
			return nil, errors.New("No core or token")
		}
		if err := checkSourceToken(inst, source); err != nil {
			return nil, err
		}
		source.ClientID = params.Get("client_id")
		if source.ClientID == "" {
			return nil, errors.New("No client_id")
		}
		source.ClientSecret = params.Get("client_secret")
		if source.ClientSecret == "" {
			return nil, errors.New("No client_secret")
		}
	} else {
		if err := checkSourceCode(inst, code); err != nil {
			return nil, err
		}
		client, err := CreateRequestClient(inst)
		if err != nil {
			return nil, err
		}
		token, err := client.CreateJWT(inst, consts.AccessTokenAudience, MoveScope)
		if err != nil {
			return nil, err
		}
		source.ClientID = client.ClientID
		source.ClientSecret = client.ClientSecret
		source.Token = token
	}

	var target RequestCredentials
	cozyURL := params.Get("target_url")
	if cozyURL == "" {
		return nil, errors.New("No target_url")
	}
	target.Token = params.Get("target_token")
	if target.Token == "" {
		return nil, errors.New("No target_token")
	}
	target.ClientID = params.Get("target_client_id")
	if target.ClientID == "" {
		return nil, errors.New("No target_client_id")
	}
	target.ClientSecret = params.Get("target_client_secret")
	if target.ClientSecret == "" {
		return nil, errors.New("No target_client_secret")
	}

	req := &Request{
		SourceCreds: source,
		TargetCreds: target,
		Target:      cozyURL,
	}

	secret, err := getStore().Save(inst, req)
	if err != nil {
		return nil, err
	}

	req.Link = inst.PageURL("/move/go", url.Values{"secret": {secret}})
	return req, nil
}

func checkSourceToken(inst *instance.Instance, source RequestCredentials) error {
	var claims permission.Claims
	err := crypto.ParseJWT(source.Token, func(token *jwt.Token) (interface{}, error) {
		return consts.AccessTokenAudience, nil
	}, &claims)
	if err != nil {
		return permission.ErrInvalidToken
	}

	if claims.Issuer != inst.Domain {
		return permission.ErrInvalidToken
	}
	if claims.Expired() {
		return permission.ErrExpiredToken
	}

	c, err := oauth.FindClient(inst, claims.Subject)
	if err != nil {
		if couchdb.IsInternalServerError(err) {
			return err
		}
		return permission.ErrInvalidToken
	}

	if c.ClientID != source.ClientID {
		return permission.ErrInvalidToken
	}
	if c.ClientSecret != source.ClientSecret {
		return permission.ErrInvalidToken
	}
	return nil
}

func checkSourceCode(inst *instance.Instance, code string) error {
	accessCode := &oauth.AccessCode{}
	if err := couchdb.GetDoc(inst, consts.OAuthAccessCodes, code, accessCode); err != nil {
		return permission.ErrInvalidToken
	}
	if accessCode.ClientID != SourceClientID {
		return permission.ErrInvalidToken
	}
	if accessCode.Scope != consts.ExportsRequests {
		return permission.ErrInvalidToken
	}
	return nil
}
