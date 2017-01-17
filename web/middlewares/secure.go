package middlewares

import (
	"fmt"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/labstack/echo"
)

type (
	// XFrameOption type for the values of the X-Frame-Options header.
	XFrameOption string

	// CSPSource type are the different types of CSP headers sources definitions.
	// Each source type defines a different acess policy.
	CSPSource int

	// SecureConfig defines the config for Secure middleware.
	SecureConfig struct {
		HSTSMaxAge    time.Duration
		CSPScriptSrc  []CSPSource
		CSPFrameSrc   []CSPSource
		CSPConnectSrc []CSPSource
		XFrameOptions XFrameOption
		XFrameAllowed string
	}
)

const (
	// XFrameDeny is the DENY option of the X-Frame-Options header.
	XFrameDeny XFrameOption = "DENY"
	// XFrameSameOrigin is the SAMEORIGIN option of the X-Frame-Options header.
	XFrameSameOrigin = "SAMEORIGIN"
	// XFrameAllowFrom is the ALLOW-FROM option of the X-Frame-Options header. It
	// should be used along with the XFrameAllowed field of SecureConfig.
	XFrameAllowFrom = "ALLOW-FROM"

	// CSPSrcSelf is the 'self' option of a CSP source.
	CSPSrcSelf CSPSource = iota
	// CSPSrcParent adds the parent domain as an eligible CSP source.
	CSPSrcParent
	// CSPSrcParentSubdomains add all the parent's subdomains as eligibles CSP
	// sources.
	CSPSrcParentSubdomains
	// CSPSrcAny is the '*' option. It allows any domain as an eligible source.
	CSPSrcAny
)

// Secure returns a Middlefunc that can be used to define all the necessary
// secure headers. It is configurable with a SecureConfig object.
func Secure(conf *SecureConfig) echo.MiddlewareFunc {
	var hstsHeader string
	if conf.HSTSMaxAge > 0 {
		hstsHeader = fmt.Sprintf("max-age=%.f; includeSubdomains",
			conf.HSTSMaxAge.Seconds())
	}

	var xFrameHeader string
	switch conf.XFrameOptions {
	case XFrameDeny:
		xFrameHeader = string(XFrameDeny)
	case XFrameSameOrigin:
		xFrameHeader = string(XFrameSameOrigin)
	case XFrameAllowFrom:
		xFrameHeader = fmt.Sprintf("%s %s", XFrameAllowFrom, conf.XFrameAllowed)
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			hsts := true
			if in := c.Get("instance"); in != nil && in.(*instance.Instance).Dev {
				hsts = false
			}
			h := c.Response().Header()
			if hsts && hstsHeader != "" {
				h.Set(echo.HeaderStrictTransportSecurity, hstsHeader)
			}
			if xFrameHeader != "" {
				h.Set(echo.HeaderXFrameOptions, xFrameHeader)
			}
			var cspHeader string
			host := c.Request().Host
			if len(conf.CSPScriptSrc) > 0 {
				cspHeader += makeCSPHeader(host, "script-src", conf.CSPScriptSrc)
			}
			if len(conf.CSPFrameSrc) > 0 {
				cspHeader += makeCSPHeader(host, "frame-src", conf.CSPFrameSrc)
			}
			if len(conf.CSPConnectSrc) > 0 {
				cspHeader += makeCSPHeader(host, "connect-src", conf.CSPConnectSrc)
			}
			if cspHeader != "" {
				h.Set(echo.HeaderContentSecurityPolicy, cspHeader)
			}
			return next(c)
		}
	}
}

func makeCSPHeader(host, header string, sources []CSPSource) string {
	headers := make([]string, len(sources))
	for i, src := range sources {
		switch src {
		case CSPSrcSelf:
			headers[i] = "'self'"
		case CSPSrcParent:
			parent, _ := SplitHost(host)
			headers[i] = parent
		case CSPSrcParentSubdomains:
			parent, _ := SplitHost(host)
			headers[i] = "*." + parent
		case CSPSrcAny:
			headers[i] = "*"
		}
	}
	return header + " " + strings.Join(headers, " ") + ";"
}
