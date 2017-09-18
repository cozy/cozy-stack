package sessions

import (
	"errors"
	"net/http"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/labstack/echo"
)

// SessionCookieName : name of the cookie created by cozy
const SessionCookieName = "cozysessid"

// SessionContextKey name of the session in echo.Context
const SessionContextKey = "session"

// SessionMaxAge : duration of the session
const SessionMaxAge = 7 * 24 * 60 * 60
const maxAgeDuration = SessionMaxAge * time.Second

var (
	// ErrNoCookie is returned by GetSession if there is no cookie
	ErrNoCookie = errors.New("No session cookie")
	// ErrInvalidID is returned by GetSession if the cookie contains wrong ID
	ErrInvalidID = errors.New("Session cookie has wrong ID")
)

// A Session is an instance opened in a browser
type Session struct {
	Instance *instance.Instance `json:"-"`
	DocID    string             `json:"_id,omitempty"`
	DocRev   string             `json:"_rev,omitempty"`
	LastSeen time.Time          `json:"last_seen,omitempty"`
}

// DocType implements couchdb.Doc
func (s *Session) DocType() string { return consts.Sessions }

// ID implements couchdb.Doc
func (s *Session) ID() string { return s.DocID }

// SetID implements couchdb.Doc
func (s *Session) SetID(v string) { s.DocID = v }

// Rev implements couchdb.Doc
func (s *Session) Rev() string { return s.DocRev }

// SetRev implements couchdb.Doc
func (s *Session) SetRev(v string) { s.DocRev = v }

// Clone implements couchdb.Doc
func (s *Session) Clone() couchdb.Doc {
	cloned := *s
	if cloned.Instance != nil {
		tmp := *s.Instance
		cloned.Instance = &tmp
	}
	return &cloned
}

// ensure Session implements couchdb.Doc
var _ couchdb.Doc = (*Session)(nil)

// OlderThan check if a session last seen is older than t from now
func (s *Session) OlderThan(t time.Duration) bool {
	return time.Now().After(s.LastSeen.Add(t))
}

// New creates a session in couchdb for the given instance
func New(i *instance.Instance) (*Session, error) {
	var s = &Session{
		Instance: i,
		LastSeen: time.Now(),
	}

	if err := couchdb.CreateDoc(i, s); err != nil {
		return nil, err
	}
	getCache().Set(i.Domain, s.DocID, s)
	return s, nil
}

// GetSession retrieves the session from a echo.Context
func GetSession(c echo.Context, i *instance.Instance) (*Session, error) {
	var err error
	// check for cached session in context
	si := c.Get(SessionContextKey)
	if si != nil {
		if sp, ok := si.(*Session); ok {
			return sp, nil
		}
	}

	cookie, err := c.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil, ErrNoCookie
	}

	sessionID, err := crypto.DecodeAuthMessage(cookieMACConfig(i), []byte(cookie.Value))
	if err != nil {
		return nil, err
	}

	updateCache := false
	s := getCache().Get(i.Domain, string(sessionID))
	if s == nil {
		s = &Session{}
		err = couchdb.GetDoc(i, consts.Sessions, string(sessionID), s)
		// invalid session id
		if couchdb.IsNotFoundError(err) {
			return nil, ErrInvalidID
		}
		if err != nil {
			return nil, err
		}
		updateCache = true
	}

	// if the session is older than half its maxAgeDuration,
	// save the new LastSeen
	if s.OlderThan(maxAgeDuration / 2) {
		s.LastSeen = time.Now()
		err := couchdb.UpdateDoc(i, s)
		if err != nil {
			i.Logger().Warn("[session] Failed to update session last seen:", err)
		}
		updateCache = true
	}

	if updateCache {
		getCache().Set(i.Domain, s.DocID, s)
	}

	c.Set(SessionContextKey, s)
	return s, nil
}

// Delete is a function to delete the session in couchdb,
// and returns a cookie with a negative MaxAge to clear it
func (s *Session) Delete(i *instance.Instance) *http.Cookie {
	getCache().Revoke(i.Domain, s.DocID)
	err := couchdb.DeleteDoc(i, s)
	if err != nil {
		i.Logger().Error("[session] Failed to delete session:", err)
	}
	return &http.Cookie{
		Name:   SessionCookieName,
		Value:  "",
		MaxAge: -1,
		Path:   "/",
		Domain: utils.StripPort("." + i.Domain),
	}
}

// ToCookie returns an http.Cookie for this Session
func (s *Session) ToCookie() (*http.Cookie, error) {
	encoded, err := crypto.EncodeAuthMessage(cookieMACConfig(s.Instance), []byte(s.ID()))
	if err != nil {
		return nil, err
	}

	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    string(encoded),
		MaxAge:   SessionMaxAge,
		Path:     "/",
		Domain:   utils.StripPort("." + s.Instance.Domain),
		Secure:   !s.Instance.Dev,
		HttpOnly: true,
	}, nil
}

// ToAppCookie returns an http.Cookie for this Session on an app subdomain
func (s *Session) ToAppCookie(domain string) (*http.Cookie, error) {
	encoded, err := crypto.EncodeAuthMessage(cookieMACConfig(s.Instance), []byte(s.ID()))
	if err != nil {
		return nil, err
	}

	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    string(encoded),
		MaxAge:   86400, // 1 day
		Path:     "/",
		Domain:   utils.StripPort(domain),
		Secure:   !s.Instance.Dev,
		HttpOnly: true,
	}, nil
}

// cookieMACConfig returns the options to authenticate the session cookie.
//
// We rely on a MACed cookie value, without additional encryption of the
// message since it should not contain critical private informations and is
// protected by HTTPs (secure cookie).
//
// About MaxLength, for a session of size 100 bytes
//
//       8 bytes time
//   +  32 bytes HMAC-SHA256
//   + 100 bytes session
//   + base64 encoding (4*n/3)
//   < 200 bytes
//
// 256 bytes should be sufficient enough to support any type of session.
//
func cookieMACConfig(i *instance.Instance) *crypto.MACConfig {
	return &crypto.MACConfig{
		Name:   SessionCookieName,
		Key:    i.SessionSecret,
		MaxAge: SessionMaxAge,
		MaxLen: 256,
	}
}
