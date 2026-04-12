package webdav

import (
	"sync"
)

// deadPropKey uniquely identifies a dead property instance on a specific
// resource in a specific Cozy instance.
type deadPropKey struct {
	domain    string // Cozy instance domain (from request Host header)
	vfsPath   string // absolute VFS path (e.g. /litmus/prop)
	namespace string // XML namespace URI (e.g. http://example.com/neon/litmus/)
	local     string // XML local name (e.g. nonesuch)
}

// deadPropValue holds the XML text value of a stored dead property.
type deadPropValue struct {
	rawXML string // the inner XML content of the property element (text or XML subtree)
}

// deadPropStore is an in-memory store for dead (custom) WebDAV properties.
//
// This implements the minimal dead-property storage required for the litmus
// `props` suite. Properties are stored in memory only — they are NOT persisted
// to CouchDB and will be lost on server restart. Full persistent dead-property
// storage is a v2 requirement (ADV-V2-02).
//
// Thread safety: protected by a single RWMutex because litmus tests are
// sequential and the store is small. A sharded approach would be needed for
// production-scale concurrent writes.
var deadPropStore = &deadProps{
	m: make(map[deadPropKey]deadPropValue),
}

type deadProps struct {
	mu sync.RWMutex
	m  map[deadPropKey]deadPropValue
}

// set stores or overwrites a dead property value.
func (dp *deadProps) set(key deadPropKey, value string) {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	dp.m[key] = deadPropValue{rawXML: value}
}

// remove deletes a dead property. It is a no-op if the property does not exist.
func (dp *deadProps) remove(key deadPropKey) {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	delete(dp.m, key)
}

// get returns the stored value and true if the property exists, or ("", false).
func (dp *deadProps) get(key deadPropKey) (string, bool) {
	dp.mu.RLock()
	defer dp.mu.RUnlock()
	v, ok := dp.m[key]
	return v.rawXML, ok
}

// listFor returns all dead properties stored for (domain, vfsPath) as a
// slice of (key, value) pairs. Used by handlePropfind to append dead
// properties to live properties in the PROPFIND response.
func (dp *deadProps) listFor(domain, vfsPath string) []struct {
	k deadPropKey
	v string
} {
	dp.mu.RLock()
	defer dp.mu.RUnlock()
	var result []struct {
		k deadPropKey
		v string
	}
	for k, v := range dp.m {
		if k.domain == domain && k.vfsPath == vfsPath {
			result = append(result, struct {
				k deadPropKey
				v string
			}{k, v.rawXML})
		}
	}
	return result
}

// clearForPath removes all dead properties stored for (domain, vfsPath).
// Called during resource DELETE and MOVE to clean up orphaned properties.
func (dp *deadProps) clearForPath(domain, vfsPath string) {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	for k := range dp.m {
		if k.domain == domain && k.vfsPath == vfsPath {
			delete(dp.m, k)
		}
	}
}

// movePropsForPath moves all dead properties from oldPath to newPath.
// Called during resource MOVE so properties follow the resource.
func (dp *deadProps) movePropsForPath(domain, oldPath, newPath string) {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	toMove := make(map[deadPropKey]deadPropValue)
	for k, v := range dp.m {
		if k.domain == domain && k.vfsPath == oldPath {
			toMove[k] = v
		}
	}
	for k, v := range toMove {
		delete(dp.m, k)
		k.vfsPath = newPath
		dp.m[k] = v
	}
}
