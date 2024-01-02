package instance

import (
	"fmt"
	"net/url"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/manager"
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

// ManagerURL returns an external string for the given ManagerURL kind. It is
// used for redirecting the user to a manager URL.
func (i *Instance) ManagerURL(k ManagerURLKind) (string, error) {
	if i.UUID == "" {
		return "", nil
	}

	config, ok := i.SettingsContext()
	if !ok {
		return "", nil
	}

	base, ok := config["manager_url"].(string)
	if !ok {
		return "", nil
	}

	baseURL, err := url.Parse(base)
	if err != nil {
		return "", err
	}

	var path string
	switch k {
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

// APIManagerClient returns a client to talk to the manager via its API.
func APIManagerClient(inst *Instance) *manager.APIClient {
	clouderies := config.GetConfig().Clouderies
	if clouderies == nil {
		return nil
	}

	var cloudery config.ClouderyConfig
	cloudery, ok := clouderies[inst.ContextName]
	if !ok {
		cloudery, ok = clouderies[config.DefaultInstanceContext]
	}
	if !ok {
		return nil
	}

	api := cloudery.API
	if api.URL == "" || api.Token == "" {
		return nil
	}

	return manager.NewAPIClient(api.URL, api.Token)
}
