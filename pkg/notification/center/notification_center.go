package center

import (
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/notification"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/workers/mails"
	"github.com/cozy/cozy-stack/pkg/workers/push"
	multierror "github.com/hashicorp/go-multierror"
)

// Push creates and send a new notification in database. This method verifies
// the permissions associated with this creation in order to check that it is
// granted to create a notification and to extract its source.
func Push(inst *instance.Instance, perm *permissions.Permission, n *notification.Notification) error {
	if n.Title == "" {
		return ErrBadNotification
	}

	var p *notification.Properties
	switch perm.Type {
	case permissions.TypeOauth:
		c, ok := perm.Client.(*oauth.Client)
		if !ok {
			return ErrUnauthorized
		}
		if c.Notifications != nil {
			p = c.Notifications[n.Category]
		}
		if p == nil {
			return ErrUnauthorized
		}
		n.Originator = "oauth"
	case permissions.TypeWebapp:
		slug := strings.TrimPrefix(perm.SourceID, consts.Apps+"/")
		m, err := apps.GetWebappBySlug(inst, slug)
		if err != nil {
			return err
		}
		if m.Notifications != nil {
			p = m.Notifications[n.Category]
		}
		n.Originator = "app"
	default:
		return ErrUnauthorized
	}

	// XXX: for retro-compatibility, we do not yet block applications from
	// sending notification from unknown category.
	if p != nil && p.Stateful {
		l, err := findLastNotification(inst, n.Source())
		if err != nil {
			return err
		}
		// when the state is the same for the last notification from this source,
		// we do not bother sending or creating a new notification.
		if l != nil && l.State == n.State {
			return nil
		}
	}

	preferredChannels := n.PreferredChannels
	if len(preferredChannels) == 0 {
		preferredChannels = []string{"mail"}
	}

	n.NID = ""
	n.NRev = ""
	n.SourceID = n.Source()
	n.CreatedAt = time.Now()
	n.PreferredChannels = nil

	if err := couchdb.CreateDoc(inst, n); err != nil {
		return err
	}

	var errm error
	for _, channel := range preferredChannels {
		switch channel {
		case "mobile":
			if p != nil {
				if err := sendPush(inst, p.Collapsible, n); err != nil {
					errm = multierror.Append(errm, err)
				}
			}
		case "mail":
			if err := sendMail(inst, n); err != nil {
				errm = multierror.Append(errm, err)
			}
		}
	}
	return errm
}

func findLastNotification(inst *instance.Instance, source string) (*notification.Notification, error) {
	var notifs []*notification.Notification
	req := &couchdb.FindRequest{
		UseIndex: "by-source-id",
		Selector: mango.Equal("source", source),
		Sort: mango.SortBy{
			{Field: "source_id", Direction: mango.Desc},
			{Field: "created_at", Direction: mango.Desc},
		},
		Limit: 1,
	}
	err := couchdb.FindDocs(inst, consts.Notifications, req, &notifs)
	if err != nil {
		return nil, err
	}
	if len(notifs) == 0 {
		return nil, nil
	}
	return notifs[0], nil
}

func sendPush(inst *instance.Instance, collapsible bool, n *notification.Notification) error {
	push := push.Message{
		NotificationID: n.ID(),
		Source:         n.Source(),
		Title:          n.Title,
		Message:        n.Message,
		Priority:       n.Priority,
		Sound:          n.Sound,
		Data:           n.Data,
		Collapsible:    collapsible,
	}
	msg, err := jobs.NewMessage(&push)
	if err != nil {
		return err
	}
	_, err = jobs.System().PushJob(&jobs.JobRequest{
		Domain:     inst.Domain,
		WorkerType: "push",
		Message:    msg,
	})
	return err
}

func sendMail(inst *instance.Instance, n *notification.Notification) error {
	parts := []*mails.Part{
		{Body: n.Content, Type: "text/plain"},
	}
	if n.ContentHTML != "" {
		parts = append(parts, &mails.Part{Body: n.ContentHTML, Type: "text/html"})
	}
	mail := mails.Options{
		Mode:    mails.ModeNoReply,
		Subject: n.Title,
		Parts:   parts,
	}
	msg, err := jobs.NewMessage(&mail)
	if err != nil {
		return err
	}
	_, err = jobs.System().PushJob(&jobs.JobRequest{
		Domain:     inst.Domain,
		WorkerType: "sendmail",
		Message:    msg,
	})
	return err
}
