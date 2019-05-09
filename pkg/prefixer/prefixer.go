package prefixer

// Prefixer interface describes a handle for a specific instance by its domain
// and a specific and unique prefix.
type Prefixer interface {
	DBPrefix() string
	DomainName() string
}

type prefixer struct {
	domain string
	prefix string
}

// UnknownDomainName represents the human-readable string of an empty domain
// name of a prefixer struct
const UnknownDomainName string = "<unknown>"

func (p *prefixer) DBPrefix() string { return p.prefix }

func (p *prefixer) DomainName() string {
	if p.domain == "" {
		return UnknownDomainName
	}
	return p.domain
}

// NewPrefixer returns a prefixer with the specified domain and prefix values.
func NewPrefixer(domain, prefix string) Prefixer {
	return &prefixer{
		domain: domain,
		prefix: prefix,
	}
}

// GlobalPrefixer returns a global prefixer with the wildcard '*' as prefix.
var GlobalPrefixer = NewPrefixer("", "global")
var ConceptIndexorPrefixer = NewPrefixer("", "conceptindexor")
var TargetPrefixer = NewPrefixer("", "target")
var TargetFinderPrefixer = NewPrefixer("", "targetfinder")
var DataAggregatorPrefixer = NewPrefixer("", "dataaggregator")
var ConductorPrefixer = NewPrefixer("", "conductor")
