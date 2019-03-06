package consts

// AppType is an enum to represent the type of application: webapp clientside
// or konnector serverside.
type AppType int

const (
	// WebappType is the clientside application type
	WebappType AppType = iota + 1
	// KonnectorType is the serverside application type
	KonnectorType
)

// String returns the human-readable doctype from the AppType
func (at AppType) String() string {
	switch at {
	case WebappType:
		return "io.cozy.apps"
	case KonnectorType:
		return "io.cozy.konnectors"
	default:
		return "unknown"
	}
}

// NewAppType creates a new AppType from a string
func NewAppType(doctype string) AppType {
	switch doctype {
	case "io.cozy.konnectors":
		return KonnectorType
	case "io.cozy.apps":
		return WebappType
	default:
		return 0
	}
}
