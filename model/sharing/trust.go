package sharing

import (
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/utils"
)

// IsTrustedMember checks if a member is trusted for auto-accepting sharings.
// Trust is determined by domain matching:
// - The member's domain matches the local instance domain (or is a subdomain)
// - The member's domain matches any configured trusted domains (or subdomains)
//
// TODO: Add contact-based trust - auto-trust contacts who we've accepted shares from before
func IsTrustedMember(inst *instance.Instance, member *Member) bool {
	if inst == nil || member == nil {
		return false
	}
	options := config.GetConfig().Sharing.OptionsForContext(inst.ContextName)

	if options.AutoAcceptTrusted == nil || !*options.AutoAcceptTrusted {
		return false
	}

	// Extract the member's instance domain
	memberDomain := utils.ExtractInstanceHost(member.Instance)
	if memberDomain == "" {
		return false
	}

	// Check if member's domain matches any trusted domain
	for _, domain := range options.TrustedDomains {
		if domain == "" {
			continue
		}
		d := utils.NormalizeDomain(domain)
		if d == "" {
			continue
		}
		if memberDomain == d || strings.HasSuffix(memberDomain, "."+d) {
			return true
		}
	}

	return false
}
