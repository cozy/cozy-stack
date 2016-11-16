package auth

import (
	"errors"
	"net/http"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/instance"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
)

// SessionsType : The couchdb type for a session
const SessionsType = "io.cozy.sessions"

// SessionCookieName : name of the cookie created by cozy
const SessionCookieName = "cozysessid"

// SessionContextKey name of the session in gin.Context
const SessionContextKey = "session"

// SessionMaxAge : duration of the session
const SessionMaxAge = 7 * 24 * 60 * 60
const maxAgeDuration = SessionMaxAge * time.Second

var (
	// ErrNoCookie is returned by GetSession if there is no cookie
	ErrNoCookie = errors.New("No session cookie")
	// ErrInvalidID is returned by GetSession if the cookie contains wrong ID
	ErrInvalidID = errors.New("Session cookie has wrong ID")
	// ErrExpired is returned by GetSession if the cookie's session is old
	ErrExpired = errors.New("Session cookie has wrong ID")
)

// A Session is an instance opened in a browser
type Session struct {
	Instance *instance.Instance `json:"-"`
	DocID    string             `json:"_id,omitempty"`
	DocRev   string             `json:"_rev,omitempty"`
	LastSeen time.Time          `json:"last_seen,omitempty"`
	Closed   bool               `json:"closed"`
}

// DocType implements couchdb.Doc
func (s *Session) DocType() string { return SessionsType }

// ID implements couchdb.Doc
func (s *Session) ID() string { return s.DocID }

// SetID implements couchdb.Doc
func (s *Session) SetID(v string) { s.DocID = v }

// Rev implements couchdb.Doc
func (s *Session) Rev() string { return s.DocRev }

// SetRev implements couchdb.Doc
func (s *Session) SetRev(v string) { s.DocRev = v }

// ensure Session implements couchdb.Doc
var _ couchdb.Doc = (*Session)(nil)

// OlderThan check if a session last seen is older than t from now
func (s *Session) OlderThan(t time.Duration) bool {
	return time.Now().After(s.LastSeen.Add(t))
}

// NewSession creates a session in couchdb for the given instance
func NewSession(i *instance.Instance) (*Session, error) {
	var s = &Session{
		Instance: i,
		LastSeen: time.Now(),
		Closed:   false,
	}

	return s, couchdb.CreateDoc(i, s)
}

// GetSession retrieves the session from a gin.Context
func GetSession(c *gin.Context) (*Session, error) {
	var s Session
	var err error
	// check for cached session in context
	if si, ok := c.Get(SessionContextKey); ok {
		if sp, ok := si.(*Session); ok {
			return sp, nil
		}
	}

	i := middlewares.GetInstance(c)
	sid, _ := c.Cookie(SessionCookieName)

	// no cookie
	if sid == "" {
		return nil, ErrNoCookie
	}

	err = couchdb.GetDoc(i, SessionsType, sid, &s)
	// invalid session id
	if couchdb.IsNotFoundError(err) {
		return nil, ErrInvalidID
	}

	if err != nil {
		return nil, err
	}

	// expired session
	if s.OlderThan(maxAgeDuration) {
		return nil, ErrExpired
	}

	// if the session is older than half its maxAgeDuration,
	// save the new LastSeen
	if s.OlderThan(maxAgeDuration / 2) {
		s.LastSeen = time.Now()
		err := couchdb.UpdateDoc(i, &s)
		if err != nil {
			log.Warn("Failed to update session last seen.")
		}
	}

	c.Set(SessionContextKey, &s)

	return &s, nil

}

// ToCookie returns an http.Cookie for this Session
// TODO SECURITY figure out if we keep session in couchdb or not
// if we do, check if ID is random enough on the whole server to use as a
// sessionid
func (s *Session) ToCookie() *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    s.ID(),
		MaxAge:   SessionMaxAge,
		Path:     "/",
		Domain:   "." + s.Instance.Domain,
		Secure:   true,
		HttpOnly: true,
	}
}
