package sharing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/mail"
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
	invite := &InviteMsg{
		Sharer:      sharer,
		Description: desc,
	}

	for i, m := range s.Members {
		if i == 0 || m.Status != MemberStatusMailNotSent { //i == 0 is for the owner
			continue
		}
		link := m.MailLink(inst, s, s.Credentials[i-1].State, codes)
		if m.Email == "" {
			invite.Link = link
			if err := m.CallInvite(inst, invite); err != nil {
				inst.Logger().WithField("nspace", "sharing").
					Errorf("Can't call invite for %#v: %s", m.Instance, err)
				continue
			}
		} else {
			if err := m.SendMail(inst, s, sharer, desc, link); err != nil {
				inst.Logger().WithField("nspace", "sharing").
					Errorf("Can't send email for %#v: %s", m.Email, err)
				return ErrMailNotSent
			}
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
		key := m.Email
		if key == "" {
			key = m.Instance
		}
		link := m.MailLink(inst, s, states[key], nil)
		if err := m.SendMail(inst, s, sharer, desc, link); err != nil {
			inst.Logger().WithField("nspace", "sharing").
				Errorf("Can't send email for %#v: %s", m.Email, err)
			return ErrMailNotSent
		}
		for i, member := range s.Members {
			if i == 0 {
				continue // skip the owner
			}
			var found bool
			if m.Email == "" {
				found = m.Instance == member.Instance
			} else {
				found = m.Email == member.Email
			}
			if found && member.Status == MemberStatusMailNotSent {
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
	addr := &mail.Address{
		Email: m.Email,
		Name:  m.PrimaryName(),
	}
	mailValues := map[string]interface{}{
		"RecipientName":    addr.Name,
		"SharerPublicName": sharer,
		"Description":      description,
		"SharingLink":      link,
	}
	msg, err := job.NewMessage(mail.Options{
		Mode:           "from",
		To:             []*mail.Address{addr},
		TemplateName:   "sharing_request",
		TemplateValues: mailValues,
		RecipientName:  addr.Name,
		Layout:         mail.CozyCloudLayout,
	})
	if err != nil {
		return err
	}
	_, err = job.System().PushJob(inst, &job.JobRequest{
		WorkerType: "sendmail",
		Message:    msg,
	})
	return err
}

// InviteMsg is the struct for calling the invite route
type InviteMsg struct {
	Sharer      string `json:"sharer_public_name"`
	Description string `json:"description"`
	Link        string `json:"sharing_link"`
}

// CallInvite sends the HTTP request to the cozy of the recipient for sending
// the invitation by mail.
func (m *Member) CallInvite(inst *instance.Instance, invite *InviteMsg) error {
	u, err := url.Parse(m.Instance)
	if err != nil {
		return err
	}
	body, err := json.Marshal(invite)
	if err != nil {
		return err
	}
	opts := &request.Options{
		Method: http.MethodPost,
		Scheme: u.Scheme,
		Domain: u.Host,
		Path:   "/sharings/invite",
		Body:   bytes.NewReader(body),
	}
	res, err := request.Req(opts)
	if err != nil {
		return err
	}
	res.Body.Close()
	return nil
}

// SendInviteMail will send an invitation email to the owner of this cozy.
func SendInviteMail(inst *instance.Instance, invite *InviteMsg) error {
	name, _ := inst.PublicName()
	if name == "" {
		name = inst.Translate("Sharing Empty name")
	}
	mailValues := map[string]interface{}{
		"RecipientName":    name,
		"SharerPublicName": invite.Sharer,
		"Description":      invite.Description,
		"SharingLink":      invite.Link,
	}
	msg, err := job.NewMessage(mail.Options{
		Mode:           "noreply",
		TemplateName:   "sharing_request",
		TemplateValues: mailValues,
		Layout:         mail.CozyCloudLayout,
	})
	if err != nil {
		return err
	}
	_, err = job.System().PushJob(inst, &job.JobRequest{
		WorkerType: "sendmail",
		Message:    msg,
	})
	return err
}
