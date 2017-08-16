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
	// CSPSrcWS adds the parent domain eligible for websocket.
	CSPSrcWS
	// CSPSrcSiblings adds all the siblings subdomains as eligibles CSP
	// sources.
	CSPSrcSiblings
	// CSPSrcAny is the '*' option. It allows any domain as an eligible source.
	CSPSrcAny
	// CSPUnsafeInline is the  'unsafe-inline' option. It allows to have inline
	// styles or scripts to be injected in the page.
	CSPUnsafeInline
	// CSPSrcWhitelist enables the whitelist just below in CSP.
	CSPSrcWhitelist

	// CSPWhitelist is a whitelist of domains that are allowed in CSP. It's not
	// permanent, this whitelist will be removed when we will have a more
	// generic way to enable client-side apps to access some domains (proxy).
	CSPWhitelist = "piwik.cozycloud.cc *.tile.openstreetmap.org *.tile.osm.org *.tiles.mapbox.com api.mapbox.com"
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
			parent, _, siblings := SplitHost(c.Request().Host)
			if len(conf.CSPDefaultSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "default-src", conf.CSPDefaultSrc)
			}
			if len(conf.CSPScriptSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "script-src", conf.CSPScriptSrc)
			}
			if len(conf.CSPFrameSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "frame-src", conf.CSPFrameSrc)
			}
			if len(conf.CSPConnectSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "connect-src", conf.CSPConnectSrc)
			}
			if len(conf.CSPFontSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "font-src", conf.CSPFontSrc)
			}
			if len(conf.CSPImgSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "img-src", conf.CSPImgSrc)
			}
			if len(conf.CSPManifestSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "manifest-src", conf.CSPManifestSrc)
			}
			if len(conf.CSPMediaSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "media-src", conf.CSPMediaSrc)
			}
			if len(conf.CSPObjectSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "object-src", conf.CSPObjectSrc)
			}
			if len(conf.CSPStyleSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "style-src", conf.CSPStyleSrc)
			}
			if len(conf.CSPWorkerSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "worker-src", conf.CSPWorkerSrc)
			}
			if cspHeader != "" {
				h.Set(echo.HeaderContentSecurityPolicy, cspHeader)
			}
			h.Set(echo.HeaderXContentTypeOptions, "nosniff")
			return next(c)
		}
	}
}

func makeCSPHeader(parent, siblings, header string, sources []CSPSource) string {
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
		case CSPSrcWS:
			headers[i] = "ws://" + parent + " wss://" + parent
		case CSPSrcSiblings:
			headers[i] = siblings
		case CSPSrcAny:
			headers[i] = "*"
		case CSPUnsafeInline:
			headers[i] = "'unsafe-inline'"
		case CSPSrcWhitelist:
			headers[i] = CSPWhitelist
		}
	}
	return header + " " + strings.Join(headers, " ") + ";"
}
