package sessions

import (
	"net/http"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/mssola/user_agent"
)

// LoginEntry stores informations associated with a new login. It is useful to
// provide the user with informations about the history of all the logins that
// may have happened on its domain.
type LoginEntry struct {
	DocID     string    `json:"_id,omitempty"`
	DocRev    string    `json:"_rev,omitempty"`
	IP        string    `json:"ip"`
	OS        string    `json:"os"`
	UA        string    `json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
}

// DocType implements couchdb.Doc
func (l *LoginEntry) DocType() string { return consts.SessionsLogins }

// ID implements couchdb.Doc
func (l *LoginEntry) ID() string { return l.DocID }

// SetID implements couchdb.Doc
func (l *LoginEntry) SetID(v string) { l.DocID = v }

// Rev implements couchdb.Doc
func (l *LoginEntry) Rev() string { return l.DocRev }

// SetRev implements couchdb.Doc
func (l *LoginEntry) SetRev(v string) { l.DocRev = v }

// Clone implements couchdb.Doc
func (l *LoginEntry) Clone() couchdb.Doc {
	clone := *l
	return &clone
}

// UserAgent returns the parsed user agent associated with the login entry.
func (l *LoginEntry) UserAgent() *user_agent.UserAgent {
	return user_agent.New(l.UA)
}

// StoreNewLoginEntry creates a new login entry in the database associated with
// the given instance.
func StoreNewLoginEntry(i *instance.Instance, req *http.Request) error {
	var ip string
	if forwardedFor := req.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		ip = strings.TrimSpace(strings.SplitN(forwardedFor, ",", 2)[0])
	}
	if ip == "" {
		ip = req.RemoteAddr
	}

	ua := user_agent.New(req.UserAgent())

	l := &LoginEntry{
		IP:        ip,
		OS:        ua.OS(),
		UA:        req.UserAgent(),
		CreatedAt: time.Now(),
	}

	return couchdb.CreateDoc(i, l)
}
