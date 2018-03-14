package center

import (
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/globals"
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

	// XXX Retro-compatible notifications with content/content_html fields.
	retroCompatMode := n.Content != "" || n.ContentHTML != ""

	var p *notification.Properties
	if !retroCompatMode {

		switch perm.Type {
		// Applications and services have TypeWebapp permissions
		case permissions.TypeWebapp:
			slug := strings.TrimPrefix(perm.SourceID, consts.Apps)
			man, err := apps.GetWebappBySlug(inst, slug)
			if err != nil {
				return err
			}
			var ok bool
			p, ok = man.Notifications[n.Category]
			if !ok {
				return ErrUnauthorized
			}
		default:
			return ErrUnauthorized
		}

		if p.Stateful {
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
	}

	preferedChannel := n.PreferedChannel

	n.NID = ""
	n.NRev = ""
	n.SourceID = n.Source()
	n.CreatedAt = time.Now()
	n.PreferedChannel = ""

	if err := couchdb.CreateDoc(inst, n); err != nil {
		return err
	}

	if !retroCompatMode && preferedChannel == "mobile" {
		return sendPush(inst, p.Collapsible, n)
	}
	return sendMail(inst, n)
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
	clients, err := oauth.GetNotifiables(inst)
	if err != nil {
		return err
	}
	var errm error
	for _, c := range clients {
		push := push.Message{
			Title:       n.Title,
			Message:     n.Message,
			Priority:    n.Priority,
			Sound:       n.Sound,
			Collapsible: collapsible,
			Platform:    c.NotificationPlatform,
			DeviceToken: c.NotificationDeviceToken,
		}
		msg, err := jobs.NewMessage(&push)
		if err != nil {
			return err
		}
		_, err = globals.GetBroker().PushJob(&jobs.JobRequest{
			Domain:     inst.Domain,
			WorkerType: "push",
			Message:    msg,
		})
		if err != nil {
			errm = multierror.Append(errm, err)
		}
	}
	return errm
}

func sendMail(inst *instance.Instance, n *notification.Notification) error {
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
	if err != nil {
		return err
	}
	_, err = globals.GetBroker().PushJob(&jobs.JobRequest{
		Domain:     inst.Domain,
		WorkerType: "sendmail",
		Message:    msg,
	})
	return err
}
