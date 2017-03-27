package config

import (
	"fmt"
	"net/url"

	"github.com/ncw/swift"
)

// NewSwiftConnection returns a swift.Connection pointer created from the
// actual configuration.
func NewSwiftConnection(fsURL *url.URL) (conn *swift.Connection, err error) {
	q := fsURL.Query()

	var authURL *url.URL
	auth := q.Get("AuthURL")
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
		username = q.Get("UserName")
		password = q.Get("Password")
	} else {
		password = q.Get("Token")
	}

	conn = &swift.Connection{
		UserName:       username,
		ApiKey:         password,
		AuthUrl:        authURL.String(),
		Domain:         q.Get("UserDomainName"),
		Tenant:         q.Get("ProjectName"),
		TenantId:       q.Get("ProjectID"),
		TenantDomain:   q.Get("ProjectDomain"),
		TenantDomainId: q.Get("ProjectDomainID"),
	}

	return conn, nil
}
