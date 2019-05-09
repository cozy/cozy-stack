package lifecycle

import (
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// GetInstance retrieves the instance for a request by its host.
func GetInstance(domain string) (*instance.Instance, error) {
	var err error
	domain, err = validateDomain(domain)
	if err != nil {
		return nil, err
	}
	i, err := instance.GetFromCouch(domain)
	if err != nil {
		return nil, err
	}

	// This retry-loop handles the probability to hit an Update conflict from
	// this version update, since the instance document may be updated different
	// processes at the same time.
	for {
		if i == nil {
			i, err = instance.GetFromCouch(domain)
			if err != nil {
				return nil, err
			}
		}

		if i.IndexViewsVersion == couchdb.IndexViewsVersion {
			break
		}

		i.Logger().Debugf("Indexes outdated: wanted %d; got %d", couchdb.IndexViewsVersion, i.IndexViewsVersion)
		if err = defineViewsAndIndex(i); err != nil {
			i.Logger().Errorf("Could not re-define indexes and views: %s", err.Error())
			return nil, err
		}

		// Copy over the instance object some data that we used to store on the
		// settings document.
		if i.TOSSigned == "" || i.UUID == "" || i.ContextName == "" {
			var settings *couchdb.JSONDoc
			settings, err = i.SettingsDocument()
			if err != nil {
				return nil, err
			}
			i.UUID, _ = settings.M["uuid"].(string)
			i.TOSSigned, _ = settings.M["tos"].(string)
			i.ContextName, _ = settings.M["context"].(string)
			// TOS version number were YYYYMMDD dates, before we used a semver-like
			// version scheme. We consider them to be the versions 1.0.0.
			if len(i.TOSSigned) == 8 {
				i.TOSSigned = "1.0.0-" + i.TOSSigned
			}
		}

		if err = update(i); err == nil {
			break
		}

		if !couchdb.IsConflictError(err) {
			return nil, err
		}

		i = nil
	}

	if err = i.MakeVFS(); err != nil {
		return nil, err
	}
	return i, nil
}
