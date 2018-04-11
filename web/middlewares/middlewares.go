package middlewares

import (
	"strings"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/echo"
)

// Compose can be used to compose a list of middlewares together with a main
// handler function. It returns a new handler that should be the composition of
// all the middlwares with the initial handler.
func Compose(handler echo.HandlerFunc, mws ...echo.MiddlewareFunc) echo.HandlerFunc {
	for i := len(mws) - 1; i >= 0; i-- {
		handler = mws[i](handler)
	}
	return handler
}

// SplitHost returns a splitted host domain taking into account the subdomains
// configuration mode used.
func SplitHost(host string) (instanceHost, appSlug, siblings string) {
	parts := strings.SplitN(host, ".", 2)
	if len(parts) == 2 {
		if config.GetConfig().Subdomains == config.NestedSubdomains {
			return parts[1], parts[0], "*." + parts[1]
		}
		subs := strings.SplitN(parts[0], "-", 2)
		if len(subs) == 2 {
			return subs[0] + "." + parts[1], subs[1], "*." + parts[1]
		}
		return host, "", ""
	}
	return parts[0], "", ""
}
