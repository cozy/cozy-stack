package middlewares

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/echo"
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

		CSPDefaultSrcWhitelist string

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

	// cspTilesWhitelist is a whitelist of tiles domains that are allowed in CSP.
	// It's not permanent, this whitelist will be removed when we will have a
	// more generic way to enable client-side apps to access some domains
	// (proxy).
	cspTilesWhitelist = "https://piwik.cozycloud.cc https://*.tile.openstreetmap.org " +
		"https://*.tile.osm.org https://*.tiles.mapbox.com https://api.mapbox.com"
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

	conf.CSPDefaultSrcWhitelist = validCSPList(conf.CSPDefaultSrcWhitelist)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			isSecure := true
			if in := c.Get("instance"); in != nil && in.(*instance.Instance).Dev {
				isSecure = false
			}
			h := c.Response().Header()
			if isSecure && hstsHeader != "" {
				h.Set(echo.HeaderStrictTransportSecurity, hstsHeader)
			}
			if xFrameHeader != "" {
				h.Set(echo.HeaderXFrameOptions, xFrameHeader)
			}
			var cspHeader string
			parent, _, siblings := SplitHost(c.Request().Host)
			if len(conf.CSPDefaultSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "default-src", conf.CSPDefaultSrcWhitelist, conf.CSPDefaultSrc, isSecure)
			}
			if len(conf.CSPScriptSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "script-src", "", conf.CSPScriptSrc, isSecure)
			}
			if len(conf.CSPFrameSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "frame-src", "", conf.CSPFrameSrc, isSecure)
			}
			if len(conf.CSPConnectSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "connect-src", "", conf.CSPConnectSrc, isSecure)
			}
			if len(conf.CSPFontSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "font-src", "", conf.CSPFontSrc, isSecure)
			}
			if len(conf.CSPImgSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "img-src", cspTilesWhitelist, conf.CSPImgSrc, isSecure)
			}
			if len(conf.CSPManifestSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "manifest-src", "", conf.CSPManifestSrc, isSecure)
			}
			if len(conf.CSPMediaSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "media-src", "", conf.CSPMediaSrc, isSecure)
			}
			if len(conf.CSPObjectSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "object-src", "", conf.CSPObjectSrc, isSecure)
			}
			if len(conf.CSPStyleSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "style-src", "", conf.CSPStyleSrc, isSecure)
			}
			if len(conf.CSPWorkerSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "worker-src", "", conf.CSPWorkerSrc, isSecure)
			}
			if cspHeader != "" {
				h.Set(echo.HeaderContentSecurityPolicy, cspHeader)
			}
			h.Set(echo.HeaderXContentTypeOptions, "nosniff")
			return next(c)
		}
	}
}

func validCSPList(list string) string {
	fields := strings.Fields(list)
	filter := fields[:0]
	for _, s := range fields {
		u, err := url.Parse(s)
		if err != nil {
			continue
		}
		u.Scheme = "https"
		if u.Path == "" {
			u.Path = "/"
		}
		filter = append(filter, u.String())
	}
	return strings.Join(filter, " ")
}

func makeCSPHeader(parent, siblings, header, cspWhitelist string, sources []CSPSource, isSecure bool) string {
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
			if isSecure {
				headers[i] = "https://" + parent
			} else {
				headers[i] = "http://" + parent
			}
		case CSPSrcWS:
			if isSecure {
				headers[i] = "wss://" + parent
			} else {
				headers[i] = "ws://" + parent
			}
		case CSPSrcSiblings:
			if isSecure {
				headers[i] = "https://" + siblings
			} else {
				headers[i] = "http://" + siblings
			}
		case CSPSrcAny:
			headers[i] = "*"
		case CSPUnsafeInline:
			headers[i] = "'unsafe-inline'"
		}
	}
	if cspWhitelist != "" {
		headers = append(headers, cspWhitelist)
	}
	return header + " " + strings.Join(headers, " ") + ";"
}
