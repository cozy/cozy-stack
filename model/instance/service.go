package instance

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cozy/cozy-stack/pkg/cache"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/lock"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"golang.org/x/net/idna"
)

const cachePrefix = "i:"
const lockNamespace = "indexes"

type InstanceService struct {
	cache       cache.Cache
	logger      *logger.Entry
	indexLocker lock.Getter
}

func NewService(cache cache.Cache, lock lock.Getter) *InstanceService {
	return &InstanceService{
		cache:       cache,
		logger:      logger.WithNamespace("instance"),
		indexLocker: lock,
	}
}

// Get finds an instance from its domain by using CouchDB or the cache.
//
// It also check if the valid indexes/views are used. If a change have been
// detected it will update all the indexes/views before returning the instance.
func (s *InstanceService) Get(ctx context.Context, rawDomain string) (*Instance, error) {
	cacheMiss := false

	domain, err := ValidateDomain(rawDomain)
	if err != nil {
		return nil, err
	}

	inst := s.getFromCache(domain)
	if inst == nil {
		cacheMiss = true
		inst, err = s.getFromCouch(domain)
	}

	if err != nil {
		return nil, err
	}

	if inst.IndexViewsVersion != couchdb.IndexViewsVersion {
		l := s.indexLocker.ReadWrite(inst, lockNamespace)

		if err := l.Lock(); err != nil {
			return nil, fmt.Errorf("failed to take the lock: %w", err)
		}
		defer l.Unlock()

		// Some indexes and/or views have changed since the last call. Updates
		// all the views/indexes.
		s.logger.WithDomain(inst.Domain).Warnf("Update Indexes")

		err := couchdb.UpdateIndexesAndViews(inst, couchdb.Indexes, couchdb.Views)
		if err != nil {
			return nil, fmt.Errorf("failed to update indexes: %w", err)
		}

		inst.IndexViewsVersion = couchdb.IndexViewsVersion

		if err = inst.Update(); err == nil {
			return nil, fmt.Errorf("failed to update the infex views version: %w", err)
		}

		cacheMiss = true
	}

	if err = inst.MakeVFS(); err != nil {
		return nil, fmt.Errorf("failed to make the vfs: %w", err)
	}

	if cacheMiss {
		if data, err := json.Marshal(inst); err == nil {
			s.cache.SetNX(inst.cacheKey(), data, cacheTTL)
		}
	}
	return inst, nil
}

func (s *InstanceService) getFromCache(domain string) *Instance {
	var inst Instance

	data, ok := s.cache.Get(cachePrefix + domain)
	if !ok {
		return nil
	}

	if err := json.Unmarshal(data, &inst); err != nil {
		return nil
	}

	return &inst
}

func (s *InstanceService) getFromCouch(domain string) (*Instance, error) {
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

	return inst, nil
}

const illegalChars = " /,;&?#@|='\"\t\r\n\x00"
const illegalFirstChars = "0123456789."

func ValidateDomain(domain string) (string, error) {
	var err error
	if domain, err = idna.ToUnicode(domain); err != nil {
		return "", ErrIllegalDomain
	}
	domain = strings.TrimSpace(domain)
	if domain == "" || domain == ".." || domain == "." {
		return "", ErrIllegalDomain
	}
	if strings.ContainsAny(domain, illegalChars) {
		return "", ErrIllegalDomain
	}
	if strings.ContainsAny(domain[:1], illegalFirstChars) {
		return "", ErrIllegalDomain
	}
	domain = strings.ToLower(domain)
	if config.GetConfig().Subdomains == config.FlatSubdomains {
		parts := strings.SplitN(domain, ".", 2)
		if strings.Contains(parts[0], "-") {
			return "", ErrIllegalDomain
		}
	}
	return domain, nil
}
