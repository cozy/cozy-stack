package notification

import (
	"fmt"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// Properties is a notification type parameters, describing how a specific
// notification group should behave.
type Properties struct {
	Description     string            `json:"description,omitempty"`
	Collapsible     bool              `json:"collapsible,omitempty"`
	Multiple        bool              `json:"multiple,omitempty"`
	Stateful        bool              `json:"stateful,omitempty"`
	DefaultPriority string            `json:"default_priority,omitempty"`
	TimeToLive      time.Duration     `json:"time_to_live,omitempty"`
	Templates       map[string]string `json:"templates,omitempty"`
	MinInterval     time.Duration     `json:"min_interval,omitempty"`

	MailTemplate string `json:"-"`
}

// Clone returns a cloned Properties struct pointer.
func (p *Properties) Clone() *Properties {
	cloned := *p
	cloned.Templates = make(map[string]string, len(p.Templates))
	for k, v := range p.Templates {
		cloned.Templates[k] = v
	}
	return &cloned
}

// Notification data containing associated to an application a list of actions
type Notification struct {
	NID  string `json:"_id,omitempty"`
	NRev string `json:"_rev,omitempty"`

	SourceID   string `json:"source_id"`
	Originator string `json:"originator,omitempty"`
	Slug       string `json:"slug,omitempty"`
	Category   string `json:"category"`
	CategoryID string `json:"category_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	LastSent  time.Time `json:"last_sent"`

	Title    string                 `json:"title,omitempty"`
	Message  string                 `json:"message,omitempty"`
	Priority string                 `json:"priority,omitempty"`
	Sound    string                 `json:"sound,omitempty"`
	State    interface{}            `json:"state,omitempty"`
	Data     map[string]interface{} `json:"data,omitempty"`

	PreferredChannels []string `json:"preferred_channels,omitempty"`

	// XXX retro-compatible fields for sending rich mail
	Content     string `json:"content,omitempty"`
	ContentHTML string `json:"content_html,omitempty"`
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
	cloned.Data = make(map[string]interface{}, len(n.Data))
	for k, v := range n.Data {
		cloned.Data[k] = v
	}
	cloned.PreferredChannels = make([]string, len(n.PreferredChannels))
	copy(cloned.PreferredChannels, n.PreferredChannels)
	return &cloned
}

// SetID is used to implement the couchdb.Doc interface
func (n *Notification) SetID(id string) { n.NID = id }

// SetRev is used to implement the couchdb.Doc interface
func (n *Notification) SetRev(rev string) { n.NRev = rev }

// Match implements permissions.Matcher
func (n *Notification) Match(k, f string) bool { return false }

// Source returns the complete normalized source value. This should be recorded
// in the `source_id` field.
func (n *Notification) Source() string {
	return fmt.Sprintf("cozy/%s/%s/%s/%s",
		n.Originator,
		n.Slug,
		n.Category,
		n.CategoryID)
}
