package instance

import (
	"encoding/json"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

const cacheTTL = 5 * time.Minute
const cachePrefix = "i:"

func (inst *Instance) cacheKey() string {
	return cachePrefix + inst.Domain
}

// Get finds an instance from its domain by using CouchDB or the cache.
func Get(domain string) (*Instance, error) {
	cache := config.GetConfig().CacheStorage
	if data, ok := cache.Get(cachePrefix + domain); ok {
		inst := &Instance{}
		err := json.Unmarshal(data, inst)
		if err == nil && inst.MakeVFS() == nil {
			return inst, nil
		}
	}
	inst, err := GetFromCouch(domain)
	if err != nil {
		return nil, err
	}
	if data, err := json.Marshal(inst); err == nil {
		cache.SetNX(inst.cacheKey(), data, cacheTTL)
	}
	return inst, nil
}

// GetFromCouch finds an instance in CouchDB from its domain.
func GetFromCouch(domain string) (*Instance, error) {
	var res couchdb.ViewResponse
	err := couchdb.ExecView(prefixer.GlobalPrefixer, couchdb.DomainAndAliasesView, &couchdb.ViewRequest{
		Key:         domain,
		IncludeDocs: true,
		Limit:       1,
	}, &res)
	if couchdb.IsNoDatabaseError(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(res.Rows) == 0 {
		return nil, ErrNotFound
	}
	inst := &Instance{}
	err = json.Unmarshal(res.Rows[0].Doc, inst)
	if err != nil {
		return nil, err
	}
	if err = inst.MakeVFS(); err != nil {
		return nil, err
	}
	return inst, nil
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
