package sharing

import (
	"fmt"
	"net/url"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/workers/mails"
)

// SendMails sends invitation mails to the recipients that were in the
// mail-not-sent status (owner only)
func (s *Sharing) SendMails(inst *instance.Instance, codes map[string]string) error {
	if !s.Owner {
		return ErrInvalidSharing
	}
	if len(s.Members) != len(s.Credentials)+1 {
		return ErrInvalidSharing
	}
	sharer, desc := s.getSharerAndDescription(inst)

	for i, m := range s.Members {
		if i == 0 || m.Status != MemberStatusMailNotSent { //i == 0 is for the owner
			continue
		}
		link := m.MailLink(inst, s, s.Credentials[i-1].State, codes)
		if err := m.SendMail(inst, s, sharer, desc, link); err != nil {
			inst.Logger().WithField("nspace", "sharing").
				Errorf("Can't send email for %#v: %s", m.Email, err)
			return ErrMailNotSent
		}
		s.Members[i].Status = MemberStatusPendingInvitation
	}

	return couchdb.UpdateDoc(inst, s)
}

// SendMailsToMembers sends mails from a recipient (open_sharing) to their
// contacts to invite them
func (s *Sharing) SendMailsToMembers(inst *instance.Instance, members []Member, states map[string]string) error {
	sharer, desc := s.getSharerAndDescription(inst)
	for _, m := range members {
		link := m.MailLink(inst, s, states[m.Email], nil)
		if err := m.SendMail(inst, s, sharer, desc, link); err != nil {
			inst.Logger().WithField("nspace", "sharing").
				Errorf("Can't send email for %#v: %s", m.Email, err)
			return ErrMailNotSent
		}
		for i, member := range s.Members {
			if i == 0 {
				continue // skip the owner
			}
			if m.Email == member.Email && member.Status == MemberStatusMailNotSent {
				s.Members[i].Status = MemberStatusPendingInvitation
				break
			}
		}
	}
	return couchdb.UpdateDoc(inst, s)
}

func (s *Sharing) getSharerAndDescription(inst *instance.Instance) (string, string) {
	sharer, _ := inst.PublicName()
	if sharer == "" {
		sharer = inst.Translate("Sharing Empty name")
	}
	desc := s.Description
	if desc == "" {
		desc = inst.Translate("Sharing Empty description")
	}
	return sharer, desc
}

// MailLink generates an HTTP link where the recipient can start the process of
// accepting the sharing
func (m *Member) MailLink(inst *instance.Instance, s *Sharing, state string, codes map[string]string) string {
	if s.Owner && s.PreviewPath != "" && codes != nil {
		if code, ok := codes[m.Email]; ok {
			u := inst.SubDomain(s.AppSlug)
			u.Path = s.PreviewPath
			u.RawQuery = url.Values{"sharecode": {code}}.Encode()
			return u.String()
		}
	}

	query := url.Values{"state": {state}}
	path := fmt.Sprintf("/sharings/%s/discovery", s.SID)
	return inst.PageURL(path, query)
}

// SendMail sends an invitation mail to a recipient
func (m *Member) SendMail(inst *instance.Instance, s *Sharing, sharer, description, link string) error {
	addr := &mails.Address{
		Email: m.Email,
		Name:  m.PrimaryName(),
	}
	mailValues := map[string]interface{}{
		"RecipientName":    addr.Name,
		"SharerPublicName": sharer,
		"Description":      description,
		"SharingLink":      link,
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
