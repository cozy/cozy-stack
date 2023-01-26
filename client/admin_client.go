package client

import (
	"time"

	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/tlsclient"
)

type AdminClient struct {
	Client
}

func (ac *AdminClient) NewInstanceClient(domain string, scopes ...string) (*Client, error) {
	token, err := ac.GetToken(&TokenOptions{
		Domain:   domain,
		Subject:  "CLI",
		Audience: consts.CLIAudience,
		Scope:    scopes,
	})
	if err != nil {
		return nil, err
	}

	httpClient, clientURL, err := tlsclient.NewHTTPClient(tlsclient.HTTPEndpoint{
		Host:      config.GetConfig().Host,
		Port:      config.GetConfig().Port,
		Timeout:   5 * time.Minute,
		EnvPrefix: "COZY_HOST",
	})
	if err != nil {
		return nil, err
	}

	c := &Client{
		Scheme:     clientURL.Scheme,
		Addr:       clientURL.Host,
		Domain:     domain,
		Client:     httpClient,
		Authorizer: &request.BearerAuthorizer{Token: token},
	}

	return c, nil
}
