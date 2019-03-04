package instance

import (
	"fmt"
	"net/url"
)

// ManagerURLKind is an enum type for the different kinds of manager URLs.
type ManagerURLKind int

const (
	// ManagerTOSURL is the kind for changes of TOS URL.
	ManagerTOSURL ManagerURLKind = iota
	// ManagerPremiumURL is the kind for changing the account type of the
	// instance.
	ManagerPremiumURL
	// ManagerBlockedURL is the kind for a redirection of a blocked instance.
	ManagerBlockedURL
)

// ManagerURL returns an external string for the given ManagerURL kind.
func (i *Instance) ManagerURL(k ManagerURLKind) (string, error) {
	if i.UUID == "" {
		return "", nil
	}

	config, err := i.SettingsContext()
	if err != nil {
		return "", nil
	}

	base, ok := config["manager_url"]
	if !ok {
		return "", nil
	}

	baseURL, err := url.Parse(base.(string))
	if err != nil {
		return "", err
	}

	var path string
	switch k {
	// TODO: we may want to rely on the contexts to avoid hardcoding the path
	// values of these kinds.
	case ManagerPremiumURL:
		path = fmt.Sprintf("/cozy/instances/%s/premium", url.PathEscape(i.UUID))
	case ManagerTOSURL:
		path = fmt.Sprintf("/cozy/instances/%s/tos", url.PathEscape(i.UUID))
	case ManagerBlockedURL:
		path = fmt.Sprintf("/cozy/instances/%s/blocked", url.PathEscape(i.UUID))
	default:
		panic("unknown ManagerURLKind")
	}
	baseURL.Path = path

	return baseURL.String(), nil
}
