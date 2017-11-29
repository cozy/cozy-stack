package middlewares

import (
	"strings"

	"github.com/cozy/cozy-stack/pkg/config"
)

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
