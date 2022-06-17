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

	ipaddress := net.ParseIP(host)
	if ipaddress == nil {
		return fmt.Errorf("%s is not a valid IP address", host)
	}

	if isPrivateIP(ipaddress) {
		return fmt.Errorf("%s is not a public IP address", ipaddress)
	}

	// Allow loopback and custom ports for dev only (127.0.0.1 / localhost), as
	// it can be useful for accepting sharings on cozy.localhost:8080 for
	// example.
	if build.IsDevRelease() {
		return nil
	}

	if ipaddress.IsLoopback() {
		return fmt.Errorf("%s is not a public IP address", ipaddress)
	}

	if port != "80" && port != "443" {
		return fmt.Errorf("%s is not a safe port number", port)
	}

	return nil
}

// isPrivateIP reports whether ip is a private address, according to RFC 1918
// (IPv4 addresses) and RFC 4193 (IPv6 addresses).
//
// Note: this function has been copied from https://pkg.go.dev/net#IP.IsPrivate
// as it has been added for go1.17 and we still want to support go1.15.
func isPrivateIP(ip net.IP) bool {
	const IPv6len = 16

	if ip4 := ip.To4(); ip4 != nil {
		// Following RFC 1918, Section 3. Private Address Space which says:
		//   The Internet Assigned Numbers Authority (IANA) has reserved the
		//   following three blocks of the IP address space for private internets:
		//     10.0.0.0        -   10.255.255.255  (10/8 prefix)
		//     172.16.0.0      -   172.31.255.255  (172.16/12 prefix)
		//     192.168.0.0     -   192.168.255.255 (192.168/16 prefix)
		return ip4[0] == 10 ||
			(ip4[0] == 172 && ip4[1]&0xf0 == 16) ||
			(ip4[0] == 192 && ip4[1] == 168)
	}
	// Following RFC 4193, Section 8. IANA Considerations which says:
	//   The IANA has assigned the FC00::/7 prefix to "Unique Local Unicast".
	return len(ip) == IPv6len && ip[0]&0xfe == 0xfc
}
