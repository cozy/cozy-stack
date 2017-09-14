package notifications

import (
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/stack"
	"github.com/cozy/cozy-stack/pkg/workers/mails"
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
	for k, v := range n.Actions {
		cloned.Actions[k] = &(*v)
	}
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
		Data   string `json:"data"`
	}
}

// Create a new notification in database.
func Create(db couchdb.Database, sourceID string, n *Notification) error {
	if n.Content == "" || n.Title == "" {
		return ErrBadNotification
	}
	if len(n.Actions) == 0 {
		n.Actions = make([]*Action, 0)
	}
	n.Source = sourceID
	if err := couchdb.CreateDoc(db, n); err != nil {
		return err
	}
	return sendMail(db, n)
}

func sendMail(db couchdb.Database, n *Notification) error {
	mail := mails.Options{
		Mode:    mails.ModeNoReply,
		Subject: n.Title,
		Parts: []*mails.Part{
			{Body: n.Content, Type: "text/plain"},
		},
	}
	msg, err := jobs.NewMessage(jobs.JSONEncoding, &mail)
	if err != nil {
		return err
	}
	_, err = stack.GetBroker().PushJob(&jobs.JobRequest{
		Domain:     db.Prefix(),
		WorkerType: "sendmail",
		Message:    msg,
	})
	return err
}
