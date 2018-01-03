package sessions

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/labstack/echo"
)

// SessionCookieName : name of the cookie created by cozy
const SessionCookieName = "cozysessid"

const (
	// SessionMaxAge is the maximum duration of the session in seconds
	SessionMaxAge = 7 * 24 * time.Hour

	// AppCookieMaxAge is the maximum duration of the application cookie is
	// seconds
	AppCookieMaxAge = 24 * time.Hour
)

var (
	// ErrNoCookie is returned by GetSession if there is no cookie
	ErrNoCookie = errors.New("No session cookie")
	// ErrExpired is returned when the session has expired
	ErrExpired = errors.New("Session expired")
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

// Get fetch the session
func Get(i *instance.Instance, sessionID string) (*Session, error) {
	updateCache := false

	s := getCache().Get(i.Domain, sessionID)
	if s == nil {
		s = &Session{}
		err := couchdb.GetDoc(i, consts.Sessions, sessionID, s)
		if couchdb.IsNotFoundError(err) {
			return nil, ErrInvalidID
		}
		if err != nil {
			return nil, err
		}
		updateCache = true
	}
	s.Instance = i

	// if the session is older than the session max age, it has expired and
	// should be deleted.
	if s.OlderThan(SessionMaxAge) {
		err := couchdb.DeleteDoc(i, s)
		if err != nil {
			i.Logger().Warn("[session] Failed to delte expired session:", err)
		}
		if updateCache {
			getCache().Revoke(i.Domain, s.DocID)
		}
		return nil, ErrExpired
	}

	// if the session is older than half its half-life, update the LastSeen date.
	if s.OlderThan(SessionMaxAge / 2) {
		lastSeen := s.LastSeen
		s.LastSeen = time.Now()
		err := couchdb.UpdateDoc(i, s)
		if err != nil {
			i.Logger().Warn("[session] Failed to update session last seen:", err)
		} else {
			s.LastSeen = lastSeen
			updateCache = true
		}
	}

	if updateCache {
		getCache().Set(i.Domain, s.DocID, s)
	}

	return s, nil
}

// FromCookie retrieves the session from a echo.Context cookies.
func FromCookie(c echo.Context, i *instance.Instance) (*Session, error) {
	cookie, err := c.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil, ErrNoCookie
	}

	sessionID, err := crypto.DecodeAuthMessage(cookieSessionMACConfig, i.SessionSecret,
		[]byte(cookie.Value), nil)
	if err != nil {
		return nil, err
	}

	return Get(i, string(sessionID))
}

// FromAppCookie retrives the session from a application submain's cookie.
func FromAppCookie(c echo.Context, i *instance.Instance, slug string) (*Session, error) {
	if config.GetConfig().Subdomains == config.FlatSubdomains {
		cookie, err := c.Cookie(SessionCookieName)
		if err != nil || cookie.Value == "" {
			return nil, ErrNoCookie
		}

		sessionID, err := crypto.DecodeAuthMessage(cookieAppMACConfig, i.SessionSecret,
			[]byte(cookie.Value), []byte(slug))
		if err != nil {
			return nil, err
		}

		return Get(i, string(sessionID))
	}
	return FromCookie(c, i)
}

// GetAll return all active sessions
func GetAll(inst *instance.Instance) ([]*Session, error) {
	var sessions []*Session
	if err := couchdb.GetAllDocs(inst, consts.Sessions, nil, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
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
	encoded, err := crypto.EncodeAuthMessage(cookieSessionMACConfig, s.Instance.SessionSecret, []byte(s.ID()), nil)
	if err != nil {
		return nil, err
	}

	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    string(encoded),
		MaxAge:   int(SessionMaxAge.Seconds()),
		Path:     "/",
		Domain:   utils.StripPort("." + s.Instance.Domain),
		Secure:   !s.Instance.Dev,
		HttpOnly: true,
	}, nil
}

// ToAppCookie returns an http.Cookie for this Session on an app subdomain
func (s *Session) ToAppCookie(domain, slug string) (*http.Cookie, error) {
	encoded, err := crypto.EncodeAuthMessage(cookieAppMACConfig, s.Instance.SessionSecret, []byte(s.ID()), []byte(slug))
	if err != nil {
		return nil, err
	}

	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    string(encoded),
		MaxAge:   int(AppCookieMaxAge.Seconds()),
		Path:     "/",
		Domain:   utils.StripPort(domain),
		Secure:   !s.Instance.Dev,
		HttpOnly: true,
	}, nil
}

// DeleteOthers will remove all sessions except the one given in parameter.
func DeleteOthers(i *instance.Instance, selfSessionID string) error {
	var sessions []*Session
	err := couchdb.ForeachDocs(i, consts.Sessions, func(data []byte) error {
		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		sessions = append(sessions, &s)
		return nil
	})
	if err != nil {
		return err
	}
	for _, s := range sessions {
		if s.ID() != selfSessionID {
			s.Delete(i)
		}
	}
	return nil
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
var cookieAppMACConfig = crypto.MACConfig{
	Name:   SessionCookieName,
	MaxAge: SessionMaxAge,
	MaxLen: 256,
}

var cookieSessionMACConfig = crypto.MACConfig{
	Name:   SessionCookieName,
	MaxAge: AppCookieMaxAge,
	MaxLen: 256,
}
