// Package safehttp can be used for making http requests when the hostname is
// not trusted (user inputs). It will avoid SSRF by ensuring that the IP
// address which will connect is not a private address, or loopback. It also
// checks that the port is 80 or 443, not anything else.
package safehttp

import (
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"

	build "github.com/cozy/cozy-stack/pkg/config"
)

var safeDialer = &net.Dialer{
	Timeout:   30 * time.Second,
	KeepAlive: 30 * time.Second,
	DualStack: true,
	Control:   safeControl,
}

var safeTransport = &http.Transport{
	// Default values for http.DefaultClient
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           safeDialer.DialContext,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          100,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,

	// As we are connecting to a new host each time, it is better to disable
	// keep-alive
	DisableKeepAlives: true,
}

// DefaultClient is an http client that can be used instead of
// http.DefaultClient to avoid SSRF. It has the same default configuration,
// except it disabled keep-alive, as it is probably not useful in such cases.
var DefaultClient = &http.Client{
	Timeout:   10 * time.Second,
	Transport: safeTransport,
}

func safeControl(network string, address string, conn syscall.RawConn) error {
	if !(network == "tcp4" || network == "tcp6") {
		return fmt.Errorf("%s is not a safe network type", network)
	}

	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%s is not a valid host/port pair: %s", address, err)
	}

	if port != "80" && port != "443" {
		return fmt.Errorf("%s is not a safe port number", port)
	}

	ipaddress := net.ParseIP(host)
	if ipaddress == nil {
		return fmt.Errorf("%s is not a valid IP address", host)
	}

	if ipaddress.IsPrivate() {
		return fmt.Errorf("%s is not a public IP address", ipaddress)
	}

	// Allow loopback for dev only (127.0.0.1 / localhost), as it can be useful
	// for accepeting sharings for example.
	if ipaddress.IsLoopback() && !build.IsDevRelease() {
		return fmt.Errorf("%s is not a public IP address", ipaddress)
	}

	return nil
}
