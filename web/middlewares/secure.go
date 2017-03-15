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
		HSTSMaxAge     time.Duration
		CSPDefaultSrc  []CSPSource
		CSPScriptSrc   []CSPSource
		CSPFrameSrc    []CSPSource
		CSPConnectSrc  []CSPSource
		CSPFontSrc     []CSPSource
		CSPImgSrc      []CSPSource
		CSPManifestSrc []CSPSource
		CSPMediaSrc    []CSPSource
		CSPObjectSrc   []CSPSource
		CSPStyleSrc    []CSPSource
		CSPWorkerSrc   []CSPSource
		XFrameOptions  XFrameOption
		XFrameAllowed  string
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
	// CSPSrcData is the 'data:' option of a CSP source.
	CSPSrcData
	// CSPSrcBlob is the 'blob:' option of a CSP source.
	CSPSrcBlob
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
		hstsHeader = fmt.Sprintf("max-age=%.f; includeSubDomains",
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
			parent, _ := SplitHost(c.Request().Host)
			if len(conf.CSPDefaultSrc) > 0 {
				cspHeader += makeCSPHeader(parent, "default-src", conf.CSPDefaultSrc)
			}
			if len(conf.CSPScriptSrc) > 0 {
				cspHeader += makeCSPHeader(parent, "script-src", conf.CSPScriptSrc)
			}
			if len(conf.CSPFrameSrc) > 0 {
				cspHeader += makeCSPHeader(parent, "frame-src", conf.CSPFrameSrc)
			}
			if len(conf.CSPConnectSrc) > 0 {
				cspHeader += makeCSPHeader(parent, "connect-src", conf.CSPConnectSrc)
			}
			if len(conf.CSPFontSrc) > 0 {
				cspHeader += makeCSPHeader(parent, "font-src", conf.CSPFontSrc)
			}
			if len(conf.CSPImgSrc) > 0 {
				cspHeader += makeCSPHeader(parent, "img-src", conf.CSPImgSrc)
			}
			if len(conf.CSPManifestSrc) > 0 {
				cspHeader += makeCSPHeader(parent, "manifest-src", conf.CSPManifestSrc)
			}
			if len(conf.CSPMediaSrc) > 0 {
				cspHeader += makeCSPHeader(parent, "media-src", conf.CSPMediaSrc)
			}
			if len(conf.CSPObjectSrc) > 0 {
				cspHeader += makeCSPHeader(parent, "object-src", conf.CSPObjectSrc)
			}
			if len(conf.CSPStyleSrc) > 0 {
				cspHeader += makeCSPHeader(parent, "style-src", conf.CSPStyleSrc)
			}
			if len(conf.CSPWorkerSrc) > 0 {
				cspHeader += makeCSPHeader(parent, "worker-src", conf.CSPWorkerSrc)
			}
			if cspHeader != "" {
				h.Set(echo.HeaderContentSecurityPolicy, cspHeader)
			}
			h.Set(echo.HeaderXContentTypeOptions, "nosniff")
			return next(c)
		}
	}
}

func makeCSPHeader(parent, header string, sources []CSPSource) string {
	headers := make([]string, len(sources))
	for i, src := range sources {
		switch src {
		case CSPSrcSelf:
			headers[i] = "'self'"
		case CSPSrcData:
			headers[i] = "data:"
		case CSPSrcBlob:
			headers[i] = "blob:"
		case CSPSrcParent:
			headers[i] = parent
		case CSPSrcParentSubdomains:
			headers[i] = "*." + parent
		case CSPSrcAny:
			headers[i] = "*"
		}
	}
	return header + " " + strings.Join(headers, " ") + ";"
}
