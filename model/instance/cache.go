package instance

import (
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

func (inst *Instance) cacheKey() string {
	return cachePrefix + inst.Domain
}

// Delete removes the instance document in CouchDB.
func (inst *Instance) Delete() error {
	err := couchdb.DeleteDoc(prefixer.GlobalPrefixer, inst)
	cache := config.GetConfig().CacheStorage
	cache.Clear(inst.cacheKey())
	return err
}
