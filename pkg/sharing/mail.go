package sharing

import (
	"fmt"
	"net/url"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/globals"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/workers/mails"
)

// MailTemplateValues is a struct with the recipient's name, the sharer's
// public name, the description of the sharing, and the link to the sharing.
type MailTemplateValues struct {
	RecipientName    string
	SharerPublicName string
	Description      string
	SharingLink      string
}

// SendMails sends invitation mails to the recipients that were in the
// mail-not-sent status
func (s *Sharing) SendMails(inst *instance.Instance) error {
	if !s.Owner {
		return ErrInvalidSharing
	}
	if len(s.Members) != len(s.Credentials)+1 {
		return ErrInvalidSharing
	}

	sharer, _ := inst.PublicName()
	if sharer == "" {
		sharer = inst.Translate("Sharing Empty name")
	}
	desc := s.Description
	if desc == "" {
		desc = inst.Translate("Sharing Empty description")
	}

	for i, m := range s.Members {
		if i == 0 || m.Status != StatusMailNotSent { //i == 0 is for the owner
			continue
		}
		link := m.MailLink(inst, s, &s.Credentials[i-1])
		if err := m.SendMail(inst, s, sharer, desc, link); err != nil {
			inst.Logger().Errorf("[sharing] Can't send email for %#v: %s", m.Email, err)
			return ErrMailNotSent
		}
		m.Status = StatusPendingInvitation
	}

	return couchdb.UpdateDoc(inst, s)
}

// MailLink generates an HTTP link where the recipient can start the process of
// accepting the sharing
func (m *Member) MailLink(inst *instance.Instance, s *Sharing, creds *Credentials) string {
	// TODO link for preview
	query := url.Values{"state": {creds.State}}
	path := fmt.Sprintf("/sharings/%s/discovery", s.SID)
	return inst.PageURL(path, query)
}

// SendMail sends an invitation mail to a recipient
func (m *Member) SendMail(inst *instance.Instance, s *Sharing, sharer, description, link string) error {
	addr := &mails.Address{
		Email: m.Email,
		Name:  m.Name,
	}
	mailValues := &MailTemplateValues{
		RecipientName:    m.Name,
		SharerPublicName: sharer,
		Description:      description,
		SharingLink:      link,
	}
	msg, err := jobs.NewMessage(mails.Options{
		Mode:           "from",
		To:             []*mails.Address{addr},
		TemplateName:   "sharing_request",
		TemplateValues: mailValues,
		RecipientName:  m.Name,
	})
	if err != nil {
		return err
	}
	_, err = globals.GetBroker().PushJob(&jobs.JobRequest{
		Domain:     inst.Domain,
		WorkerType: "sendmail",
		Options:    nil,
		Message:    msg,
	})
	return err
}
