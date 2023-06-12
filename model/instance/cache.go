package instance

import (
	"encoding/json"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

const cacheTTL = 5 * time.Minute

func (inst *Instance) cacheKey() string {
	return cachePrefix + inst.Domain
}

// Update saves the changes in CouchDB.
func (inst *Instance) Update() error {
	if err := couchdb.UpdateDoc(prefixer.GlobalPrefixer, inst); err != nil {
		return err
	}
	cache := config.GetConfig().CacheStorage
	if data, err := json.Marshal(inst); err == nil {
		cache.Set(inst.cacheKey(), data, cacheTTL)
	}
	return nil
}

// Delete removes the instance document in CouchDB.
func (inst *Instance) Delete() error {
	err := couchdb.DeleteDoc(prefixer.GlobalPrefixer, inst)
	cache := config.GetConfig().CacheStorage
	cache.Clear(inst.cacheKey())
	return err
}
