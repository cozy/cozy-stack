package prefixer

// Prefixer interface describes a handle for a specific instance by its domain
// and a specific and unique prefix.
type Prefixer interface {
	DBCluster() int
	DBPrefix() string
	DomainName() string
}

type prefixer struct {
	cluster int
	domain  string
	prefix  string
}

// UnknownDomainName represents the human-readable string of an empty domain
// name of a prefixer struct
const UnknownDomainName string = "<unknown>"

func (p *prefixer) DBCluster() int { return p.cluster }

func (p *prefixer) DBPrefix() string { return p.prefix }

func (p *prefixer) DomainName() string {
	if p.domain == "" {
		return UnknownDomainName
	}
	return p.domain
}

// NewPrefixer returns a prefixer with the specified domain and prefix values.
func NewPrefixer(cluster int, domain, prefix string) Prefixer {
	return &prefixer{
		cluster: cluster,
		domain:  domain,
		prefix:  prefix,
	}
}

// GlobalCouchCluster is the index of the CouchDB cluster for the global and
// secrets databases.
const GlobalCouchCluster = -1

// GlobalPrefixer returns a global prefixer with the wildcard '*' as prefix.
var GlobalPrefixer = NewPrefixer(GlobalCouchCluster, "", "global")

// SecretsPrefixer is the the prefix used for db which hold
// a cozy stack secrets.
var SecretsPrefixer = NewPrefixer(GlobalCouchCluster, "", "secrets")
