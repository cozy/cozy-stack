package center

import (
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/notification"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/mail"
	multierror "github.com/hashicorp/go-multierror"
)

const (
	// NotificationDiskQuota category for sending alert when reaching 90% of disk
	// usage quota.
	NotificationDiskQuota = "disk-quota"
)

var (
	stackNotifications = map[string]*notification.Properties{
		NotificationDiskQuota: {
			Description:  "Warn about the diskquota reaching a high level",
			Collapsible:  true,
			Stateful:     true,
			MailTemplate: "notifications_diskquota",
			MinInterval:  7 * 24 * time.Hour,
		},
	}
)

func init() {
	vfs.RegisterDiskQuotaAlertCallback(func(domain string, exceeded bool) {
		i, err := lifecycle.GetInstance(domain)
		if err != nil {
			return
		}
		offersLink, err := i.ManagerURL(instance.ManagerPremiumURL)
		if err != nil {
			return
		}
		cozyDriveLink := i.SubDomain(consts.DriveSlug)
		n := &notification.Notification{
			State: exceeded,
			Data: map[string]interface{}{
				"OffersLink":    offersLink,
				"CozyDriveLink": cozyDriveLink.String(),
			},
		}
		_ = pushStack(domain, NotificationDiskQuota, n)
	})
}

func pushStack(domain string, category string, n *notification.Notification) error {
	inst, err := lifecycle.GetInstance(domain)
	if err != nil {
		return err
	}
	n.Originator = "stack"
	n.Category = category
	p := stackNotifications[category]
	if p == nil {
		return ErrCategoryNotFound
	}
	return makePush(inst, p, n)
}

// Push creates and send a new notification in database. This method verifies
// the permissions associated with this creation in order to check that it is
// granted to create a notification and to extract its source.
func Push(inst *instance.Instance, perm *permission.Permission, n *notification.Notification) error {
	if n.Title == "" {
		return ErrBadNotification
	}

	var p notification.Properties
	switch perm.Type {
	case permission.TypeOauth:
		c, ok := perm.Client.(*oauth.Client)
		if !ok || c.Notifications == nil {
			return ErrUnauthorized
		}
		p, ok = c.Notifications[n.Category]
		if !ok {
			return ErrUnauthorized
		}
		n.Originator = "oauth"
	case permission.TypeWebapp:
		slug := strings.TrimPrefix(perm.SourceID, consts.Apps+"/")
		m, err := app.GetWebappBySlug(inst, slug)
		if err != nil || m.Notifications == nil {
			return err
		}
		var ok bool
		p, ok = m.Notifications[n.Category]
		if !ok {
			return ErrUnauthorized
		}
		n.Slug = m.Slug()
		n.Originator = "app"
	case permission.TypeKonnector:
		slug := strings.TrimPrefix(perm.SourceID, consts.Apps+"/")
		m, err := app.GetKonnectorBySlug(inst, slug)
		if err != nil || m.Notifications == nil {
			return err
		}
		var ok bool
		p, ok = m.Notifications[n.Category]
		if !ok {
			return ErrUnauthorized
		}
		n.Slug = m.Slug()
		n.Originator = "konnector"
	default:
		return ErrUnauthorized
	}

	return makePush(inst, &p, n)
}

func makePush(inst *instance.Instance, p *notification.Properties, n *notification.Notification) error {
	lastSent := time.Now()
	skipNotification := false

	// XXX: for retro-compatibility, we do not yet block applications from
	// sending notification from unknown category.
	if p != nil && p.Stateful {
		last, err := findLastNotification(inst, n.Source())
		if err != nil {
			return err
		}
		// when the state is the same for the last notification from this source,
		// we do not bother sending or creating a new notification.
		if last != nil {
			if last.State == n.State {
				inst.Logger().WithField("nspace", "notifications").
					Debugf("Notification %v was not sent (collapsed by same state %s)", p, n.State)
				return nil
			}
			if p.MinInterval > 0 && time.Until(last.LastSent) <= p.MinInterval {
				skipNotification = true
			}
		}

		if p.Stateful && !skipNotification {
			if b, ok := n.State.(bool); ok && !b {
				skipNotification = true
			} else if i, ok := n.State.(int); ok && i == 0 {
				skipNotification = true
			}
		}

		if skipNotification && last != nil {
			lastSent = last.LastSent
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
	n.LastSent = lastSent
	n.PreferredChannels = nil

	if err := couchdb.CreateDoc(inst, n); err != nil {
		return err
	}
	if skipNotification {
		return nil
	}

	var errm error
	for _, channel := range preferredChannels {
		switch channel {
		case "mobile":
			if p != nil {
				if err := sendPush(inst, p, n); err != nil {
					errm = multierror.Append(errm, err)
				}
			}
		case "mail":
			if err := sendMail(inst, p, n); err != nil {
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
		Selector: mango.Equal("source_id", source),
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

func sendPush(inst *instance.Instance, p *notification.Properties, n *notification.Notification) error {
	push := PushMessage{
		NotificationID: n.ID(),
		Source:         n.Source(),
		Title:          n.Title,
		Message:        n.Message,
		Priority:       n.Priority,
		Sound:          n.Sound,
		Data:           n.Data,
		Collapsible:    p.Collapsible,
	}
	msg, err := job.NewMessage(&push)
	if err != nil {
		return err
	}
	_, err = job.System().PushJob(inst, &job.JobRequest{
		WorkerType: "push",
		Message:    msg,
	})
	return err
}

func sendMail(inst *instance.Instance, p *notification.Properties, n *notification.Notification) error {
	email := mail.Options{Mode: mail.ModeNoReply}

	// Notifications from the stack have their own mail templates defined
	if p != nil && p.MailTemplate != "" {
		email.TemplateName = p.MailTemplate
		email.TemplateValues = n.Data
	} else if n.ContentHTML != "" {
		email.Subject = n.Title
		email.Parts = make([]*mail.Part, 0, 2)
		if n.Content != "" {
			email.Parts = append(email.Parts,
				&mail.Part{Body: n.Content, Type: "text/plain"})
		}
		if n.ContentHTML != "" {
			email.Parts = append(email.Parts,
				&mail.Part{Body: n.ContentHTML, Type: "text/html"})
		}
	} else {
		return nil
	}

	msg, err := job.NewMessage(&email)
	if err != nil {
		return err
	}
	_, err = job.System().PushJob(inst, &job.JobRequest{
		WorkerType: "sendmail",
		Message:    msg,
	})
	return err
}
