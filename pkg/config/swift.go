package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/ncw/swift"
)

// NewSwiftConnection returns a swift.Connection pointer created from the
// actual configuration.
func NewSwiftConnection(fsURL *url.URL) (conn *swift.Connection, err error) {
	q := fsURL.Query()

	var authURL *url.URL
	auth := confOrEnv(q.Get("AuthURL"))
	if auth == "" {
		authURL = &url.URL{
			Scheme: "http",
			Host:   fsURL.Host,
			Path:   "/identity/v3",
		}
	} else {
		authURL, err = url.Parse(auth)
		if err != nil {
			return nil, fmt.Errorf("vfsswift: could not parse AuthURL %s", err)
		}
	}

	var username, password string
	if q.Get("UserName") != "" {
		username = confOrEnv(q.Get("UserName"))
		password = confOrEnv(q.Get("Password"))
	} else {
		password = confOrEnv(q.Get("Token"))
	}

	conn = &swift.Connection{
		UserName:       username,
		ApiKey:         password,
		AuthUrl:        authURL.String(),
		Domain:         confOrEnv(q.Get("UserDomainName")),
		Tenant:         confOrEnv(q.Get("ProjectName")),
		TenantId:       confOrEnv(q.Get("ProjectID")),
		TenantDomain:   confOrEnv(q.Get("ProjectDomain")),
		TenantDomainId: confOrEnv(q.Get("ProjectDomainID")),
	}

	return conn, nil
}

func confOrEnv(val string) string {
	if val == "" || val[0] != '$' {
		return val
	}
	return os.Getenv(strings.TrimSpace(val[1:]))
}
