package notifications

import (
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/globals"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/workers/mails"
	"github.com/cozy/cozy-stack/pkg/workers/push"
)

// Notification data containing associated to an application a list of actions
type Notification struct {
	NID         string    `json:"_id,omitempty"`
	NRev        string    `json:"_rev,omitempty"`
	Source      string    `json:"source"`
	Reference   string    `json:"reference"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	ContentHTML string    `json:"content_html"`
	Icon        string    `json:"icon"`
	Actions     []*Action `json:"actions"`
	CreatedAt   time.Time `json:"created_at"`

	// XXX: this field is temporary to test push notifications. It will be
	// removed when they are actually integrated into a notification center.
	WithPush *push.Message `json:"with_push,omitempty"`
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
		tmp := *v
		cloned.Actions[k] = &tmp
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
func Create(inst *instance.Instance, sourceID string, n *Notification) error {
	if n.Content == "" || n.Title == "" {
		return ErrBadNotification
	}
	if len(n.Actions) == 0 {
		n.Actions = make([]*Action, 0)
	}
	n.Source = sourceID
	n.CreatedAt = time.Now()
	err := couchdb.CreateDoc(inst, n)
	if err != nil {
		return err
	}

	var worker string
	var msg jobs.Message
	if n.WithPush != nil {
		worker, msg, err = sendPush(inst, n)
	} else {
		worker, msg, err = sendMail(inst, n)
	}
	if err != nil {
		return err
	}
	_, err = globals.GetBroker().PushJob(&jobs.JobRequest{
		Domain:     inst.Domain,
		WorkerType: worker,
		Message:    msg,
	})
	return err
}

func sendPush(inst *instance.Instance, n *Notification) (string, jobs.Message, error) {
	push := push.Message{
		Title: n.Title,

		ClientID:    n.WithPush.ClientID,
		Platform:    n.WithPush.Platform,
		DeviceToken: n.WithPush.DeviceToken,
		Topic:       n.WithPush.Topic,
		Message:     n.WithPush.Message,
		Priority:    n.WithPush.Priority,
		Sound:       n.WithPush.Sound,
	}
	msg, err := jobs.NewMessage(&push)
	return "push", msg, err
}

func sendMail(inst *instance.Instance, n *Notification) (string, jobs.Message, error) {
	var parts []*mails.Part
	if n.ContentHTML == "" {
		parts = []*mails.Part{
			{Body: n.Content, Type: "text/plain"},
		}
	} else {
		parts = []*mails.Part{
			{Body: n.ContentHTML, Type: "text/html"},
			{Body: n.Content, Type: "text/plain"},
		}
	}
	mail := mails.Options{
		Mode:    mails.ModeNoReply,
		Subject: n.Title,
		Parts:   parts,
	}
	msg, err := jobs.NewMessage(&mail)
	return "sendmail", msg, err
}
