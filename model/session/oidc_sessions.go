package session

import (
	"fmt"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	oidcbinding "github.com/cozy/cozy-stack/model/oidc/binding"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

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

	if err := oidcbinding.BindSession(s.OIDCProviderKey, i.Domain, s.SID, s.ID()); err != nil {
		i.Logger().WithNamespace("oidc").Warnf("Cannot bind OIDC session %s to %s: %s", s.SID, s.ID(), err)
		return
	}
	i.Logger().WithNamespace("oidc").Debugf(
		"Bound local session %s to OIDC sid %s in context %s",
		s.ID(), s.SID, s.OIDCProviderKey,
	)
	if err := cleanupDuplicateOIDCSessions(s); err != nil {
		i.Logger().WithNamespace("oidc").Warnf("Cannot cleanup duplicate OIDC session %s for %s: %s", s.SID, s.ID(), err)
	}
}

func unbindOIDCSession(i *instance.Instance, s *Session) {
	if i == nil || s == nil || s.SID == "" || s.OIDCProviderKey == "" {
		return
	}
	if err := oidcbinding.UnbindSession(s.OIDCProviderKey, i.Domain, s.SID, s.ID()); err != nil {
		i.Logger().WithNamespace("oidc").Warnf("Cannot unbind OIDC session %s from %s: %s", s.SID, s.ID(), err)
	}
}

func touchOIDCSession(i *instance.Instance, s *Session) {
	if i == nil || s == nil || s.SID == "" || s.OIDCProviderKey == "" {
		return
	}
	if err := oidcbinding.TouchSID(s.SID); err != nil {
		i.Logger().WithNamespace("oidc").Warnf("Cannot touch OIDC session %s for %s: %s", s.SID, s.ID(), err)
	}
}

// DeleteByOIDCSession deletes all local Cozy sessions bound to a provider-scoped
// OIDC session identifier. The first iteration uses the OIDC context name as
// the provider key.
func DeleteByOIDCSession(oidcProviderKey, sid string) (int, error) {
	refs, err := oidcbinding.ListSessions(oidcProviderKey, sid)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, ref := range refs {
		inst, err := lifecycle.GetInstance(ref.Domain)
		if err != nil {
			return deleted, fmt.Errorf("cannot resolve instance %s for OIDC local session binding: %w", ref.Domain, err)
		}

		s := &Session{}
		err = couchdb.GetDoc(inst, consts.Sessions, ref.SessionID, s)
		if couchdb.IsNotFoundError(err) {
			inst.Logger().WithNamespace("oidc").Warnf(
				"Dropping stale OIDC local session binding for sid %s: session %s not found",
				sid, ref.SessionID,
			)
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
			inst.Logger().WithNamespace("oidc").Warnf(
				"Dropping mismatched OIDC local session binding for sid %s: session %s has sid=%s provider=%s",
				sid, ref.SessionID, s.SID, s.OIDCProviderKey,
			)
			unbindOIDCSession(inst, &Session{
				DocID:           ref.SessionID,
				SID:             sid,
				OIDCProviderKey: ref.OIDCProviderKey,
			})
			continue
		}
		inst.Logger().WithNamespace("oidc").Debugf(
			"Deleting local session %s for OIDC sid %s in context %s",
			ref.SessionID, sid, oidcProviderKey,
		)
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
	refs, err := oidcbinding.ListSessions(current.OIDCProviderKey, current.SID)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if ref.SessionID == current.ID() {
			continue
		}

		inst, err := lifecycle.GetInstance(ref.Domain)
		if err != nil {
			return fmt.Errorf("cannot resolve instance %s for duplicate OIDC local session binding: %w", ref.Domain, err)
		}

		existing := &Session{}
		err = couchdb.GetDoc(inst, consts.Sessions, ref.SessionID, existing)
		if couchdb.IsNotFoundError(err) {
			inst.Logger().WithNamespace("oidc").Warnf(
				"Dropping duplicate OIDC local session binding for sid %s: session %s not found",
				current.SID, ref.SessionID,
			)
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
			inst.Logger().WithNamespace("oidc").Warnf(
				"Dropping mismatched duplicate OIDC local session binding for sid %s: session %s has sid=%s provider=%s",
				current.SID, ref.SessionID, existing.SID, existing.OIDCProviderKey,
			)
			unbindOIDCSession(inst, &Session{
				DocID:           ref.SessionID,
				SID:             current.SID,
				OIDCProviderKey: ref.OIDCProviderKey,
			})
			continue
		}
		inst.Logger().WithNamespace("oidc").Debugf(
			"Deleting duplicate local session %s for OIDC sid %s in context %s",
			ref.SessionID, current.SID, current.OIDCProviderKey,
		)
		existing.Delete(inst)
	}
	return nil
}
