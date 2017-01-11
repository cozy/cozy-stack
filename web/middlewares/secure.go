package middlewares

import (
	"fmt"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/labstack/echo"
)

type (
	XFrameOption string
	CSPSource    int

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
	XFrameDeny       XFrameOption = "DENY"
	XFrameSameOrigin              = "SAMEORIGIN"
	XFrameAllowFrom               = "ALLOW-FROM"

	CSPSrcSelf CSPSource = iota
	CSPSrcParent
	CSPSrcParentSubdomains
	CSPSrcAny
)

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
			headers[i] = parentHost(host)
		case CSPSrcParentSubdomains:
			if parent := parentHost(host); parent != "" {
				headers[i] = "*." + parent
			}
		case CSPSrcAny:
			headers[i] = "*"
		}
	}
	return header + " " + strings.Join(headers, " ") + ";"
}

func parentHost(host string) string {
	parts := strings.SplitN(host, ".", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}
