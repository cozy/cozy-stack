package binding

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/redis/go-redis/v9"
)

const oidcBindingTTL = 31 * 24 * time.Hour

const (
	oidcBindingKindSession     = "session"
	oidcBindingKindOAuthClient = "oauth_client"
)

type bindingRef struct {
	OIDCProviderKey string `json:"oidc_provider_key"`
	Domain          string `json:"domain"`
	SessionID       string `json:"session_id,omitempty"`
	OAuthClientID   string `json:"oauth_client_id,omitempty"`
	Kind            string `json:"kind,omitempty"`
}

// sessionRef is a binding to a local Cozy browser session.
type sessionRef struct {
	OIDCProviderKey string
	Domain          string
	SessionID       string
}

// oauthClientRef is a binding to a local Cozy OAuth client.
type oauthClientRef struct {
	OIDCProviderKey string
	Domain          string
	OAuthClientID   string
}

type store interface {
	Bind(sid string, ref bindingRef) error
	Unbind(sid string, ref bindingRef) error
	Touch(sid string) error
	List(sid string) ([]bindingRef, error)
}

var storeOnce sync.Once
var globalStore store

func getStore() store {
	storeOnce.Do(initStore)
	return globalStore
}

func initStore() {
	cli := config.GetConfig().SessionStorage
	if cli == nil {
		globalStore = &memStore{bindings: make(map[string]map[string]struct{})}
	} else {
		globalStore = &redisStore{c: cli, ctx: context.Background()}
	}
}

type memStore struct {
	mu       sync.Mutex
	bindings map[string]map[string]struct{}
}

func (s *memStore) Bind(sid string, ref bindingRef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := oidcBindingKey(sid)
	if s.bindings[key] == nil {
		s.bindings[key] = make(map[string]struct{})
	}
	member, err := marshalBindingRef(ref)
	if err != nil {
		return err
	}
	s.bindings[key][member] = struct{}{}
	return nil
}

func (s *memStore) Unbind(sid string, ref bindingRef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := oidcBindingKey(sid)
	members := s.bindings[key]
	if len(members) == 0 {
		return nil
	}
	member, err := marshalBindingRef(ref)
	if err != nil {
		return err
	}
	delete(members, member)
	if len(members) == 0 {
		delete(s.bindings, key)
	}
	return nil
}

func (s *memStore) Touch(_ string) error { return nil }

func (s *memStore) List(sid string) ([]bindingRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	members := s.bindings[oidcBindingKey(sid)]
	refs := make([]bindingRef, 0, len(members))
	for member := range members {
		ref, ok := unmarshalBindingRef(member)
		if ok {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

type redisStore struct {
	c   redis.UniversalClient
	ctx context.Context
}

func (s *redisStore) Bind(sid string, ref bindingRef) error {
	key := oidcBindingKey(sid)
	member, err := marshalBindingRef(ref)
	if err != nil {
		return err
	}
	pipe := s.c.TxPipeline()
	pipe.SAdd(s.ctx, key, member)
	pipe.Expire(s.ctx, key, oidcBindingTTL)
	_, err = pipe.Exec(s.ctx)
	return err
}

func (s *redisStore) Unbind(sid string, ref bindingRef) error {
	key := oidcBindingKey(sid)
	member, err := marshalBindingRef(ref)
	if err != nil {
		return err
	}
	return s.c.SRem(s.ctx, key, member).Err()
}

func (s *redisStore) Touch(sid string) error {
	return s.c.Expire(s.ctx, oidcBindingKey(sid), oidcBindingTTL).Err()
}

func (s *redisStore) List(sid string) ([]bindingRef, error) {
	members, err := s.c.SMembers(s.ctx, oidcBindingKey(sid)).Result()
	if err != nil {
		return nil, err
	}
	refs := make([]bindingRef, 0, len(members))
	for _, member := range members {
		ref, ok := unmarshalBindingRef(member)
		if ok {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

func BindSession(oidcProviderKey, domain, sid, sessionID string) error {
	if oidcProviderKey == "" || domain == "" || sid == "" || sessionID == "" {
		return nil
	}
	return getStore().Bind(sid, sessionBindingRef(oidcProviderKey, domain, sessionID))
}

func UnbindSession(oidcProviderKey, domain, sid, sessionID string) error {
	if oidcProviderKey == "" || domain == "" || sid == "" || sessionID == "" {
		return nil
	}
	return getStore().Unbind(sid, sessionBindingRef(oidcProviderKey, domain, sessionID))
}

func BindOAuthClient(oidcProviderKey, domain, sid, clientID string) error {
	if oidcProviderKey == "" || domain == "" || sid == "" || clientID == "" {
		return nil
	}
	return getStore().Bind(sid, oauthClientBindingRef(oidcProviderKey, domain, clientID))
}

func UnbindOAuthClient(oidcProviderKey, domain, sid, clientID string) error {
	if oidcProviderKey == "" || domain == "" || sid == "" || clientID == "" {
		return nil
	}
	return getStore().Unbind(sid, oauthClientBindingRef(oidcProviderKey, domain, clientID))
}

func TouchSID(sid string) error {
	if sid == "" {
		return nil
	}
	return getStore().Touch(sid)
}

func FindProviderKeys(sid string) ([]string, error) {
	refs, err := getStore().List(sid)
	if err != nil {
		return nil, err
	}
	keys := make(map[string]struct{})
	for _, ref := range refs {
		if ref.OIDCProviderKey != "" {
			keys[ref.OIDCProviderKey] = struct{}{}
		}
	}
	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	sort.Strings(out)
	return out, nil
}

func ListSessions(oidcProviderKey, sid string) ([]sessionRef, error) {
	refs, err := getStore().List(sid)
	if err != nil {
		return nil, err
	}
	out := make([]sessionRef, 0)
	for _, ref := range refs {
		if ref.Kind != oidcBindingKindSession {
			continue
		}
		if oidcProviderKey != "" && ref.OIDCProviderKey != oidcProviderKey {
			continue
		}
		out = append(out, sessionRef{
			OIDCProviderKey: ref.OIDCProviderKey,
			Domain:          ref.Domain,
			SessionID:       ref.SessionID,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OIDCProviderKey != out[j].OIDCProviderKey {
			return out[i].OIDCProviderKey < out[j].OIDCProviderKey
		}
		if out[i].Domain != out[j].Domain {
			return out[i].Domain < out[j].Domain
		}
		return out[i].SessionID < out[j].SessionID
	})
	return out, nil
}

func ListOAuthClients(oidcProviderKey, sid string) ([]oauthClientRef, error) {
	refs, err := getStore().List(sid)
	if err != nil {
		return nil, err
	}
	out := make([]oauthClientRef, 0)
	for _, ref := range refs {
		if ref.Kind != oidcBindingKindOAuthClient {
			continue
		}
		if oidcProviderKey != "" && ref.OIDCProviderKey != oidcProviderKey {
			continue
		}
		out = append(out, oauthClientRef{
			OIDCProviderKey: ref.OIDCProviderKey,
			Domain:          ref.Domain,
			OAuthClientID:   ref.OAuthClientID,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OIDCProviderKey != out[j].OIDCProviderKey {
			return out[i].OIDCProviderKey < out[j].OIDCProviderKey
		}
		if out[i].Domain != out[j].Domain {
			return out[i].Domain < out[j].Domain
		}
		return out[i].OAuthClientID < out[j].OAuthClientID
	})
	return out, nil
}

func oidcBindingKey(sid string) string {
	return "oidc:sid:" + sid
}

func marshalBindingRef(ref bindingRef) (string, error) {
	member, err := json.Marshal(ref)
	if err != nil {
		return "", err
	}
	return string(member), nil
}

func unmarshalBindingRef(member string) (bindingRef, bool) {
	var ref bindingRef
	if err := json.Unmarshal([]byte(member), &ref); err != nil {
		return bindingRef{}, false
	}
	if ref.OIDCProviderKey == "" || ref.Domain == "" {
		return bindingRef{}, false
	}
	switch ref.Kind {
	case oidcBindingKindSession:
		if ref.SessionID == "" {
			return bindingRef{}, false
		}
		return ref, true
	case oidcBindingKindOAuthClient:
		if ref.OAuthClientID == "" {
			return bindingRef{}, false
		}
		return ref, true
	default:
		return bindingRef{}, false
	}
}

func sessionBindingRef(oidcProviderKey, domain, sessionID string) bindingRef {
	return bindingRef{
		OIDCProviderKey: oidcProviderKey,
		Domain:          domain,
		SessionID:       sessionID,
		Kind:            oidcBindingKindSession,
	}
}

func oauthClientBindingRef(oidcProviderKey, domain, clientID string) bindingRef {
	return bindingRef{
		OIDCProviderKey: oidcProviderKey,
		Domain:          domain,
		OAuthClientID:   clientID,
		Kind:            oidcBindingKindOAuthClient,
	}
}
