package sharing

import (
	"fmt"
	"net/url"

	"github.com/cozy/cozy-stack/pkg/couchdb"
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
func (s *Sharing) SendMails(inst *instance.Instance, codes map[string]string) error {
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
		if i == 0 || m.Status != MemberStatusMailNotSent { //i == 0 is for the owner
			continue
		}
		link := m.MailLink(inst, s, &s.Credentials[i-1], codes)
		if err := m.SendMail(inst, s, sharer, desc, link); err != nil {
			inst.Logger().WithField("nspace", "sharing").
				Errorf("Can't send email for %#v: %s", m.Email, err)
			return ErrMailNotSent
		}
		m.Status = MemberStatusPendingInvitation
	}

	return couchdb.UpdateDoc(inst, s)
}

// MailLink generates an HTTP link where the recipient can start the process of
// accepting the sharing
func (m *Member) MailLink(inst *instance.Instance, s *Sharing, creds *Credentials, codes map[string]string) string {
	if s.Owner && s.PreviewPath != "" && codes != nil {
		if code, ok := codes[m.Email]; ok {
			u := inst.SubDomain(s.AppSlug)
			u.Path = s.PreviewPath
			u.RawQuery = url.Values{"sharecode": {code}}.Encode()
			return u.String()
		}
	}

	query := url.Values{"state": {creds.State}}
	path := fmt.Sprintf("/sharings/%s/discovery", s.SID)
	return inst.PageURL(path, query)
}

// SendMail sends an invitation mail to a recipient
func (m *Member) SendMail(inst *instance.Instance, s *Sharing, sharer, description, link string) error {
	addr := &mails.Address{
		Email: m.Email,
		Name:  m.PrimaryName(),
	}
	mailValues := &MailTemplateValues{
		RecipientName:    addr.Name,
		SharerPublicName: sharer,
		Description:      description,
		SharingLink:      link,
	}
	msg, err := jobs.NewMessage(mails.Options{
		Mode:           "from",
		To:             []*mails.Address{addr},
		TemplateName:   "sharing_request",
		TemplateValues: mailValues,
		RecipientName:  addr.Name,
	})
	if err != nil {
		return err
	}
	_, err = jobs.System().PushJob(inst, &jobs.JobRequest{
		WorkerType: "sendmail",
		Options:    nil,
		Message:    msg,
	})
	return err
}
