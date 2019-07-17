package config

import (
	"fmt"
	"net/url"
	"time"

	"github.com/ncw/swift"
)

var swiftConn *swift.Connection

// InitDefaultSwiftConnection initializes the default swift handler.
func InitDefaultSwiftConnection() error {
	return InitSwiftConnection(config.Fs)
}

// InitSwiftConnection initialize the global swift handler connection. This is
// not a thread-safe method.
func InitSwiftConnection(fs Fs) error {
	fsURL := fs.URL
	if fsURL.Scheme != SchemeSwift && fsURL.Scheme != SchemeSwiftSecure {
		return nil
	}

	q := fsURL.Query()
	isSecure := fsURL.Scheme == SchemeSwiftSecure

	var authURL *url.URL
	var err error
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
			panic(fmt.Sprintf("swift: could not parse AuthURL %s", err))
		}
	}
	if isSecure {
		authURL.Scheme = "https"
	}

	var username, password string
	if q.Get("UserName") != "" {
		username = q.Get("UserName")
		password = q.Get("Password")
	} else {
		password = q.Get("Token")
	}

	endpointType := swift.EndpointTypePublic
	if q.Get("EndpointType") == "internal" {
		endpointType = swift.EndpointTypeInternal
	} else if q.Get("EndpointType") == "admin" {
		endpointType = swift.EndpointTypeAdmin
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
		EndpointType:   endpointType,
		// Copying a file needs a long timeout on large files
		Transport:      fs.Transport,
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
