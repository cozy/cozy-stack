package notifications

import (
	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// Notification data containing associated to an application a list of actions
type Notification struct {
	NID       string    `json:"_id,omitempty"`
	NRev      string    `json:"_rev,omitempty"`
	Source    string    `json:"source"`
	Reference string    `json:"reference"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Icon      string    `json:"icon"`
	Actions   []*Action `json:"actions"`
}

// ID is used to implement the couchdb.Doc interface
func (n *Notification) ID() string { return n.NID }

// Rev is used to implement the couchdb.Doc interface
func (n *Notification) Rev() string { return n.NRev }

// DocType is used to implement the couchdb.Doc interface
func (n *Notification) DocType() string { return consts.Notifications }

// Clone implements couchdb.Doc
func (n *Notification) Clone() couchdb.Doc {
	cloned := *n
	cloned.Actions = make([]*Action, len(n.Actions))
	copy(cloned.Actions, n.Actions)
	return &cloned
}

// SetID is used to implement the couchdb.Doc interface
func (n *Notification) SetID(id string) { n.NID = id }

// SetRev is used to implement the couchdb.Doc interface
func (n *Notification) SetRev(rev string) { n.NRev = rev }

// Valid implements permissions.Validable
func (n *Notification) Valid(k, f string) bool { return false }

// Action describes the actions associated to a notification.
type Action struct {
	Text   string `json:"text"`
	Intent struct {
		Action string `json:"action"`
		Type   string `json:"type"`
	}
}

// Create a new notification in database.
func Create(db couchdb.Database, slug string, n *Notification) error {
	man, err := apps.GetBySlug(db, slug, apps.Webapp)
	if err != nil {
		return err
	}
	if n.Reference == "" || n.Content == "" || n.Title == "" || len(n.Actions) == 0 {
		return ErrBadNotification
	}
	n.Source = man.ID()
	return couchdb.CreateDoc(db, n)
}
