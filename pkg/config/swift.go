package config

import (
	"fmt"
	"net/url"
	"time"

	"github.com/cozy/swift"
)

var swiftConn *swift.Connection

// InitSwiftConnection initialize the global swift handler connection. This is
// not a thread-safe method.
func InitSwiftConnection(swiftURL *url.URL) error {
	q := swiftURL.Query()

	var authURL *url.URL
	var err error
	auth := q.Get("AuthURL")
	if auth == "" {
		authURL = &url.URL{
			Scheme: "http",
			Host:   swiftURL.Host,
			Path:   "/identity/v3",
		}
	} else {
		authURL, err = url.Parse(auth)
		if err != nil {
			panic(fmt.Sprintf("swift: could not parse AuthURL %s", err))
		}
	}

	var username, password string
	if q.Get("UserName") != "" {
		username = q.Get("UserName")
		password = q.Get("Password")
	} else {
		password = q.Get("Token")
	}

	swiftConn = &swift.Connection{
		UserName:       username,
		ApiKey:         password,
		AuthUrl:        authURL.String(),
		Domain:         q.Get("UserDomainName"),
		Tenant:         q.Get("ProjectName"),
		TenantId:       q.Get("ProjectID"),
		TenantDomain:   q.Get("ProjectDomain"),
		TenantDomainId: q.Get("ProjectDomainID"),
		Region:         q.Get("Region"),
		// Copying a file needs a long timeout on large files
		ConnectTimeout: 300 * time.Second,
		Timeout:        300 * time.Second,
	}

	if err = swiftConn.Authenticate(); err != nil {
		log.Errorf("Authentication failed with the OpenStack Swift server on %s",
			swiftConn.AuthUrl)
		return err
	}
	log.Infof("Successfully authenticated with server %s", swiftConn.AuthUrl)
	return nil
}

// GetSwiftConnection returns a swift.Connection pointer created from the
// actual configuration.
func GetSwiftConnection() *swift.Connection {
	if swiftConn == nil {
		panic("Called GetSwiftConnection() before InitSwiftConnection()")
	}
	return swiftConn
}
