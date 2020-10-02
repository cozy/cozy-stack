package middlewares

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/labstack/echo/v4"
	"golang.org/x/net/idna"
)

type (
	// CSPSource type are the different types of CSP headers sources definitions.
	// Each source type defines a different acess policy.
	CSPSource int

	// SecureConfig defines the config for Secure middleware.
	SecureConfig struct {
		HSTSMaxAge time.Duration

		CSPDefaultSrc     []CSPSource
		CSPScriptSrc      []CSPSource
		CSPFrameSrc       []CSPSource
		CSPConnectSrc     []CSPSource
		CSPFontSrc        []CSPSource
		CSPImgSrc         []CSPSource
		CSPManifestSrc    []CSPSource
		CSPMediaSrc       []CSPSource
		CSPObjectSrc      []CSPSource
		CSPStyleSrc       []CSPSource
		CSPWorkerSrc      []CSPSource
		CSPFrameAncestors []CSPSource

		CSPDefaultSrcAllowList     string
		CSPScriptSrcAllowList      string
		CSPFrameSrcAllowList       string
		CSPConnectSrcAllowList     string
		CSPFontSrcAllowList        string
		CSPImgSrcAllowList         string
		CSPManifestSrcAllowList    string
		CSPMediaSrcAllowList       string
		CSPObjectSrcAllowList      string
		CSPStyleSrcAllowList       string
		CSPWorkerSrcAllowList      string
		CSPFrameAncestorsAllowList string
	}
)

const (
	// CSPSrcSelf is the 'self' option of a CSP source.
	CSPSrcSelf CSPSource = iota
	// CSPSrcNone is the 'none' option. It denies all domains as an eligible
	// source.
	CSPSrcNone
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
	// CSPAllowList inserts a allowList of domains.
	CSPAllowList
)

// Secure returns a Middlefunc that can be used to define all the necessary
// secure headers. It is configurable with a SecureConfig object.
func Secure(conf *SecureConfig) echo.MiddlewareFunc {
	var hstsHeader string
	if conf.HSTSMaxAge > 0 {
		hstsHeader = fmt.Sprintf("max-age=%.f; includeSubDomains",
			conf.HSTSMaxAge.Seconds())
	}

	conf.CSPDefaultSrc, conf.CSPDefaultSrcAllowList =
		validCSPList(conf.CSPDefaultSrc, conf.CSPDefaultSrc, conf.CSPDefaultSrcAllowList)
	conf.CSPScriptSrc, conf.CSPScriptSrcAllowList =
		validCSPList(conf.CSPScriptSrc, conf.CSPDefaultSrc, conf.CSPScriptSrcAllowList)
	conf.CSPFrameSrc, conf.CSPFrameSrcAllowList =
		validCSPList(conf.CSPFrameSrc, conf.CSPDefaultSrc, conf.CSPFrameSrcAllowList)
	conf.CSPConnectSrc, conf.CSPConnectSrcAllowList =
		validCSPList(conf.CSPConnectSrc, conf.CSPDefaultSrc, conf.CSPConnectSrcAllowList)
	conf.CSPFontSrc, conf.CSPFontSrcAllowList =
		validCSPList(conf.CSPFontSrc, conf.CSPDefaultSrc, conf.CSPFontSrcAllowList)
	conf.CSPImgSrc, conf.CSPImgSrcAllowList =
		validCSPList(conf.CSPImgSrc, conf.CSPDefaultSrc, conf.CSPImgSrcAllowList)
	conf.CSPManifestSrc, conf.CSPManifestSrcAllowList =
		validCSPList(conf.CSPManifestSrc, conf.CSPDefaultSrc, conf.CSPManifestSrcAllowList)
	conf.CSPMediaSrc, conf.CSPMediaSrcAllowList =
		validCSPList(conf.CSPMediaSrc, conf.CSPDefaultSrc, conf.CSPMediaSrcAllowList)
	conf.CSPObjectSrc, conf.CSPObjectSrcAllowList =
		validCSPList(conf.CSPObjectSrc, conf.CSPDefaultSrc, conf.CSPObjectSrcAllowList)
	conf.CSPStyleSrc, conf.CSPStyleSrcAllowList =
		validCSPList(conf.CSPStyleSrc, conf.CSPDefaultSrc, conf.CSPStyleSrcAllowList)
	conf.CSPWorkerSrc, conf.CSPWorkerSrcAllowList =
		validCSPList(conf.CSPWorkerSrc, conf.CSPDefaultSrc, conf.CSPWorkerSrcAllowList)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			isSecure := !build.IsDevRelease()
			h := c.Response().Header()
			if isSecure && hstsHeader != "" {
				h.Set(echo.HeaderStrictTransportSecurity, hstsHeader)
			}
			var cspHeader string
			host, err := idna.ToUnicode(c.Request().Host)
			if err != nil {
				return err
			}
			parent, _, siblings := config.SplitCozyHost(host)
			parent, err = idna.ToASCII(parent)
			if err != nil {
				return err
			}
			if len(conf.CSPDefaultSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "default-src", conf.CSPDefaultSrcAllowList, conf.CSPDefaultSrc, isSecure)
			}
			if len(conf.CSPScriptSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "script-src", conf.CSPScriptSrcAllowList, conf.CSPScriptSrc, isSecure)
			}
			if len(conf.CSPFrameSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "frame-src", conf.CSPFrameSrcAllowList, conf.CSPFrameSrc, isSecure)
			}
			if len(conf.CSPConnectSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "connect-src", conf.CSPConnectSrcAllowList, conf.CSPConnectSrc, isSecure)
			}
			if len(conf.CSPFontSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "font-src", conf.CSPFontSrcAllowList, conf.CSPFontSrc, isSecure)
			}
			if len(conf.CSPImgSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "img-src", conf.CSPImgSrcAllowList, conf.CSPImgSrc, isSecure)
			}
			if len(conf.CSPManifestSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "manifest-src", conf.CSPManifestSrcAllowList, conf.CSPManifestSrc, isSecure)
			}
			if len(conf.CSPMediaSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "media-src", conf.CSPMediaSrcAllowList, conf.CSPMediaSrc, isSecure)
			}
			if len(conf.CSPObjectSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "object-src", conf.CSPObjectSrcAllowList, conf.CSPObjectSrc, isSecure)
			}
			if len(conf.CSPStyleSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "style-src", conf.CSPStyleSrcAllowList, conf.CSPStyleSrc, isSecure)
			}
			if len(conf.CSPWorkerSrc) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "worker-src", conf.CSPWorkerSrcAllowList, conf.CSPWorkerSrc, isSecure)
			}
			if len(conf.CSPFrameAncestors) > 0 {
				cspHeader += makeCSPHeader(parent, siblings, "frame-ancestors", conf.CSPFrameAncestorsAllowList, conf.CSPFrameAncestors, isSecure)
			}
			if cspHeader != "" {
				h.Set(echo.HeaderContentSecurityPolicy, cspHeader)
			}
			h.Set(echo.HeaderXContentTypeOptions, "nosniff")
			return next(c)
		}
	}
}

func validCSPList(sources, defaults []CSPSource, allowList string) ([]CSPSource, string) {
	allowListFields := strings.Fields(allowList)
	allowListFilter := allowListFields[:0]
	for _, s := range allowListFields {
		u, err := url.Parse(s)
		if err != nil {
			continue
		}
		if !build.IsDevRelease() {
			if u.Scheme == "ws" {
				u.Scheme = "wss"
			} else if u.Scheme == "http" {
				u.Scheme = "https"
			}
		}
		if u.Path == "" {
			u.Path = "/"
		}
		// For custom links like cozydrive:, we want to allow the whole protocol
		if !strings.HasPrefix(u.Scheme, "cozy") {
			s = u.String()
		}
		allowListFilter = append(allowListFilter, s)
	}

	if len(allowListFilter) > 0 {
		allowList = strings.Join(allowListFilter, " ")
		sources = append(sources, CSPAllowList)
	} else {
		allowList = ""
	}

	if len(sources) == 0 && allowList == "" {
		return nil, ""
	}

	sources = append(sources, defaults...)
	sourcesUnique := sources[:0]
	for _, source := range sources {
		var found bool
		for _, s := range sourcesUnique {
			if s == source {
				found = true
				break
			}
		}
		if !found {
			sourcesUnique = append(sourcesUnique, source)
		}
	}

	return sourcesUnique, allowList
}

func makeCSPHeader(parent, siblings, header, cspAllowList string, sources []CSPSource, isSecure bool) string {
	headers := make([]string, len(sources))
	for i, src := range sources {
		switch src {
		case CSPSrcSelf:
			headers[i] = "'self'"
		case CSPSrcNone:
			headers[i] = "'none'"
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
		case CSPAllowList:
			headers[i] = cspAllowList
		}
	}
	return header + " " + strings.Join(headers, " ") + ";"
}

// AppendCSPRule allows to patch inline the CSP headers to add a new rule.
func AppendCSPRule(c echo.Context, ruleType string, appendedValues ...string) {
	currentRules := c.Response().Header().Get(echo.HeaderContentSecurityPolicy)
	newRules := appendCSPRule(currentRules, ruleType, appendedValues...)
	c.Response().Header().Set(echo.HeaderContentSecurityPolicy, newRules)
}

func appendCSPRule(currentRules, ruleType string, appendedValues ...string) (newRules string) {
	ruleIndex := strings.Index(currentRules, ruleType)
	if ruleIndex >= 0 {
		ruleTerminationIndex := strings.Index(currentRules[ruleIndex:], ";")
		if ruleTerminationIndex <= 0 {
			return
		}
		ruleFields := strings.Fields(currentRules[ruleIndex : ruleIndex+ruleTerminationIndex])
		ruleFields = append(ruleFields, appendedValues...)
		newRules = currentRules[:ruleIndex] + strings.Join(ruleFields, " ") +
			currentRules[ruleIndex+ruleTerminationIndex:]
	} else {
		newRules = currentRules + ruleType + " " + strings.Join(appendedValues, " ") + ";"
	}
	return
}
