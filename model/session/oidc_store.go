package session

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/redis/go-redis/v9"
)

const oidcBindingTTL = SessionMaxAge + 24*time.Hour

type oidcSessionRef struct {
	OIDCProviderKey string `json:"oidc_provider_key"`
	Domain          string `json:"domain"`
	SessionID       string `json:"session_id"`
}

type oidcSessionBindingStore interface {
	Bind(oidcProviderKey, sid, domain, sessionID string) error
	Unbind(oidcProviderKey, sid, domain, sessionID string) error
	Touch(sid string) error
	List(sid string) ([]oidcSessionRef, error)
}

var oidcStoreMu sync.Mutex
var globalOIDCSessionBindingStore oidcSessionBindingStore

func getOIDCSessionBindingStore() oidcSessionBindingStore {
	oidcStoreMu.Lock()
	defer oidcStoreMu.Unlock()
	if globalOIDCSessionBindingStore != nil {
		return globalOIDCSessionBindingStore
	}
	cli := config.GetConfig().SessionStorage
	if cli == nil {
		globalOIDCSessionBindingStore = &memOIDCSessionBindingStore{
			bindings: make(map[string]map[string]struct{}),
		}
	} else {
		globalOIDCSessionBindingStore = &redisOIDCSessionBindingStore{
			c:   cli,
			ctx: context.Background(),
		}
	}
	return globalOIDCSessionBindingStore
}

type memOIDCSessionBindingStore struct {
	mu       sync.Mutex
	bindings map[string]map[string]struct{}
}

func (s *memOIDCSessionBindingStore) Bind(oidcProviderKey, sid, domain, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := oidcBindingKey(sid)
	if s.bindings[key] == nil {
		s.bindings[key] = make(map[string]struct{})
	}
	member, err := oidcSessionMember(oidcProviderKey, domain, sessionID)
	if err != nil {
		return err
	}
	s.bindings[key][member] = struct{}{}
	return nil
}

func (s *memOIDCSessionBindingStore) Unbind(oidcProviderKey, sid, domain, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := oidcBindingKey(sid)
	members := s.bindings[key]
	if len(members) == 0 {
		return nil
	}
	member, err := oidcSessionMember(oidcProviderKey, domain, sessionID)
	if err != nil {
		return err
	}
	delete(members, member)
	if len(members) == 0 {
		delete(s.bindings, key)
	}
	return nil
}

func (s *memOIDCSessionBindingStore) Touch(_ string) error {
	return nil
}

func (s *memOIDCSessionBindingStore) List(sid string) ([]oidcSessionRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	members := s.bindings[oidcBindingKey(sid)]
	refs := make([]oidcSessionRef, 0, len(members))
	for member := range members {
		ref, ok := parseOIDCSessionMember(member)
		if ok {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

type redisOIDCSessionBindingStore struct {
	c   redis.UniversalClient
	ctx context.Context
}

func (s *redisOIDCSessionBindingStore) Bind(oidcProviderKey, sid, domain, sessionID string) error {
	key := oidcBindingKey(sid)
	member, err := oidcSessionMember(oidcProviderKey, domain, sessionID)
	if err != nil {
		return err
	}
	pipe := s.c.TxPipeline()
	pipe.SAdd(s.ctx, key, member)
	pipe.Expire(s.ctx, key, oidcBindingTTL)
	_, err = pipe.Exec(s.ctx)
	return err
}

func (s *redisOIDCSessionBindingStore) Unbind(oidcProviderKey, sid, domain, sessionID string) error {
	key := oidcBindingKey(sid)
	member, err := oidcSessionMember(oidcProviderKey, domain, sessionID)
	if err != nil {
		return err
	}
	return s.c.SRem(s.ctx, key, member).Err()
}

func (s *redisOIDCSessionBindingStore) Touch(sid string) error {
	return s.c.Expire(s.ctx, oidcBindingKey(sid), oidcBindingTTL).Err()
}

func (s *redisOIDCSessionBindingStore) List(sid string) ([]oidcSessionRef, error) {
	members, err := s.c.SMembers(s.ctx, oidcBindingKey(sid)).Result()
	if err != nil {
		return nil, err
	}
	refs := make([]oidcSessionRef, 0, len(members))
	for _, member := range members {
		ref, ok := parseOIDCSessionMember(member)
		if ok {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

func oidcBindingKey(sid string) string {
	return "oidc:sid:" + sid
}

func oidcSessionMember(oidcProviderKey, domain, sessionID string) (string, error) {
	member, err := json.Marshal(oidcSessionRef{
		OIDCProviderKey: oidcProviderKey,
		Domain:          domain,
		SessionID:       sessionID,
	})
	if err != nil {
		return "", err
	}
	return string(member), nil
}

func parseOIDCSessionMember(member string) (oidcSessionRef, bool) {
	var ref oidcSessionRef
	if err := json.Unmarshal([]byte(member), &ref); err != nil {
		return oidcSessionRef{}, false
	}
	if ref.OIDCProviderKey == "" || ref.Domain == "" || ref.SessionID == "" {
		return oidcSessionRef{}, false
	}
	return ref, true
}

func bindOIDCSession(i *instance.Instance, s *Session) {
	if s == nil || s.SID == "" || s.OIDCProviderKey == "" {
		i.Logger().WithNamespace("oidc").Warnf("Cannot bind OIDC session for without SID")
		return
	}
	unlock, err := lockOIDCSessionBinding(s.OIDCProviderKey, s.SID)
	if err != nil {
		i.Logger().WithNamespace("oidc").Warnf("Cannot lock OIDC session %s for %s: %s", s.SID, s.ID(), err)
		return
	}
	defer unlock()

	if err := getOIDCSessionBindingStore().Bind(s.OIDCProviderKey, s.SID, i.Domain, s.ID()); err != nil {
		i.Logger().WithNamespace("oidc").Warnf("Cannot bind OIDC session %s to %s: %s", s.SID, s.ID(), err)
		return
	}
	if err := cleanupDuplicateOIDCSessions(s); err != nil {
		i.Logger().WithNamespace("oidc").Warnf("Cannot cleanup duplicate OIDC session %s for %s: %s", s.SID, s.ID(), err)
	}
}

func unbindOIDCSession(i *instance.Instance, s *Session) {
	if i == nil || s == nil || s.SID == "" || s.OIDCProviderKey == "" {
		return
	}
	if err := getOIDCSessionBindingStore().Unbind(s.OIDCProviderKey, s.SID, i.Domain, s.ID()); err != nil {
		i.Logger().WithNamespace("oidc").Warnf("Cannot unbind OIDC session %s from %s: %s", s.SID, s.ID(), err)
	}
}

func touchOIDCSession(i *instance.Instance, s *Session) {
	if i == nil || s == nil || s.SID == "" || s.OIDCProviderKey == "" {
		return
	}
	if err := getOIDCSessionBindingStore().Touch(s.SID); err != nil {
		i.Logger().WithNamespace("oidc").Warnf("Cannot touch OIDC session %s for %s: %s", s.SID, s.ID(), err)
	}
}

// DeleteByOIDCSession deletes all local Cozy sessions bound to a provider-scoped
// OIDC session identifier. The first iteration uses the OIDC context name as
// the provider key.
func DeleteByOIDCSession(oidcProviderKey, sid string) (int, error) {
	refs, err := getOIDCSessionBindingStore().List(sid)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, ref := range refs {
		if ref.OIDCProviderKey != oidcProviderKey {
			continue
		}
		inst, err := lifecycle.GetInstance(ref.Domain)
		if err != nil {
			// Cleanup stale bindings when the instance no longer exists.
			unbindOIDCSession(&instance.Instance{Domain: ref.Domain}, &Session{
				DocID:           ref.SessionID,
				SID:             sid,
				OIDCProviderKey: ref.OIDCProviderKey,
			})
			continue
		}

		s := &Session{}
		err = couchdb.GetDoc(inst, consts.Sessions, ref.SessionID, s)
		if couchdb.IsNotFoundError(err) {
			unbindOIDCSession(inst, &Session{
				DocID:           ref.SessionID,
				SID:             sid,
				OIDCProviderKey: ref.OIDCProviderKey,
			})
			continue
		}
		if err != nil {
			return deleted, err
		}
		if s.SID != sid || s.OIDCProviderKey != oidcProviderKey {
			unbindOIDCSession(inst, &Session{
				DocID:           ref.SessionID,
				SID:             sid,
				OIDCProviderKey: ref.OIDCProviderKey,
			})
			continue
		}
		s.Delete(inst)
		deleted++
	}
	return deleted, nil
}

func lockOIDCSessionBinding(oidcProviderKey, sid string) (func(), error) {
	mu := config.Lock().ReadWrite(prefixer.GlobalPrefixer, "oidc-session/"+oidcProviderKey+"/"+sid)
	if err := mu.Lock(); err != nil {
		return nil, err
	}
	return mu.Unlock, nil
}

func cleanupDuplicateOIDCSessions(current *Session) error {
	refs, err := getOIDCSessionBindingStore().List(current.SID)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if ref.OIDCProviderKey != current.OIDCProviderKey || ref.SessionID == current.ID() {
			continue
		}

		inst, err := lifecycle.GetInstance(ref.Domain)
		if err != nil {
			unbindOIDCSession(&instance.Instance{Domain: ref.Domain}, &Session{
				DocID:           ref.SessionID,
				SID:             current.SID,
				OIDCProviderKey: ref.OIDCProviderKey,
			})
			continue
		}

		existing := &Session{}
		err = couchdb.GetDoc(inst, consts.Sessions, ref.SessionID, existing)
		if couchdb.IsNotFoundError(err) {
			unbindOIDCSession(inst, &Session{
				DocID:           ref.SessionID,
				SID:             current.SID,
				OIDCProviderKey: ref.OIDCProviderKey,
			})
			continue
		}
		if err != nil {
			return err
		}
		if existing.SID != current.SID || existing.OIDCProviderKey != current.OIDCProviderKey {
			unbindOIDCSession(inst, &Session{
				DocID:           ref.SessionID,
				SID:             current.SID,
				OIDCProviderKey: ref.OIDCProviderKey,
			})
			continue
		}
		existing.Delete(inst)
	}
	return nil
}
