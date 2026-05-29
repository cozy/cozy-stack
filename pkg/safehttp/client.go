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

// allowedPrivateNets holds CIDR ranges that are permitted even though they are
// private/loopback addresses. Used to allow federation on private networks.
var allowedPrivateNets []*net.IPNet

// SetAllowedPrivateNetworks configures a list of CIDR ranges (e.g. "10.0.0.0/8")
// that are trusted sharing-peer networks and are allowed to be reached even when
// they are private addresses. All other SSRF protection remains active.
func SetAllowedPrivateNetworks(cidrs []string) error {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			return fmt.Errorf("safehttp: invalid CIDR %q: %w", cidr, err)
		}
		nets = append(nets, ipnet)
	}
	allowedPrivateNets = nets
	return nil
}

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

var transportWithKeepAlive = &http.Transport{
	// Default values for http.DefaultClient
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           safeDialer.DialContext,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          100,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

// ClientWithKeepAlive is an http client that can be used to avoid SSRF. And it
// has keep-alive (contrary to safehttp.DefaultClient). The typical use case is
// moving a Cozy.
var ClientWithKeepAlive = &http.Client{
	Transport: transportWithKeepAlive,
}

var loopbackDialer = &net.Dialer{
	Timeout:   30 * time.Second,
	KeepAlive: 30 * time.Second,
	DualStack: true,
}

var loopbackTransport = &http.Transport{
	// Default values for http.DefaultClient
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           loopbackDialer.DialContext,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          100,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

// LoopbackClient is an HTTP client that is allowed to connect to loopback and
// local addresses. It is used for same-stack sharing communication where the
// peer is on the same cozy-stack process and we route the request through
// 127.0.0.1 instead of the public network.
var LoopbackClient = &http.Client{
	Transport: loopbackTransport,
}

func safeControl(network string, address string, conn syscall.RawConn) error {
	if !(network == "tcp4" || network == "tcp6") {
		return fmt.Errorf("%s is not a safe network type", network)
	}

	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%s is not a valid host/port pair: %s", address, err)
	}

	ipaddress := net.ParseIP(host)
	if ipaddress == nil {
		return fmt.Errorf("%s is not a valid IP address", host)
	}

	if ipaddress.IsUnspecified() || ipaddress.IsLinkLocalUnicast() || ipaddress.IsLinkLocalMulticast() {
		return fmt.Errorf("%s is not a valid IP address", host)
	}

	if ipaddress.IsPrivate() {
		for _, allowed := range allowedPrivateNets {
			if allowed.Contains(ipaddress) {
				return nil
			}
		}
		return fmt.Errorf("%s is not a public IP address", ipaddress)
	}

	// Allow loopback and custom ports for dev only (127.0.0.1 / localhost), as
	// it can be useful for accepting sharings on cozy.localhost:8080 for
	// example.
	if build.IsDevRelease() {
		return nil
	}

	if ipaddress.IsLoopback() {
		for _, allowed := range allowedPrivateNets {
			if allowed.Contains(ipaddress) {
				return nil
			}
		}
		return fmt.Errorf("%s is not a public IP address", ipaddress)
	}

	if port != "80" && port != "443" {
		return fmt.Errorf("%s is not a safe port number", port)
	}

	return nil
}
