package sharings

import (
	"fmt"
	"net/url"

	"github.com/cozy/cozy-stack/pkg/consts"
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

// SendMails send an email to each recipient, so that they can accept or
// refuse the sharing.
func SendMails(instance *instance.Instance, s *Sharing) error {
	sharer, err := instance.PublicName()
	if err != nil || sharer == "" {
		sharer = "Someone"
	}
	desc := s.Description
	if desc == "" {
		desc = "Surprise!"
	}

	var errm error
	for _, recipient := range s.Recipients {
		if recipient.Status != consts.SharingStatusPending &&
			recipient.Status != consts.SharingStatusMailNotSent {
			continue
		}

		if err = sendMail(instance, s, &recipient, sharer, desc); err != nil {
			instance.Logger().Errorf("[sharing] Can't send email for %#v: %s", recipient.RefContact, err)
			errm = ErrRecipientHasNoEmail
			recipient.Status = consts.SharingStatusMailNotSent
			if err = couchdb.UpdateDoc(instance, s); err != nil {
				instance.Logger().Errorf("[sharing] Can't save status: %s", err)
			}
		}
	}

	return errm
}

func sendMail(i *instance.Instance, s *Sharing, m *Member, sharer, description string) error {
	contact := m.Contact(i)
	if contact == nil {
		return ErrRecipientDoesNotExist
	}
	mailAddress, err := contact.ToMailAddress()
	if err != nil {
		return err
	}
	link, err := linkForRecipient(i, s, m)
	if err != nil {
		return err
	}
	mailValues := &MailTemplateValues{
		RecipientName:    mailAddress.Name,
		SharerPublicName: sharer,
		Description:      description,
		SharingLink:      link,
	}
	msg, err := jobs.NewMessage(mails.Options{
		Mode:           "from",
		To:             []*mails.Address{mailAddress},
		TemplateName:   "sharing_request",
		TemplateValues: mailValues,
		RecipientName:  mailValues.RecipientName,
	})
	if err != nil {
		return err
	}
	_, err = globals.GetBroker().PushJob(&jobs.JobRequest{
		Domain:     i.Domain,
		WorkerType: "sendmail",
		Options:    nil,
		Message:    msg,
	})
	return err
}

func linkForRecipient(i *instance.Instance, s *Sharing, m *Member) (string, error) {
	perms, err := s.Permissions(i)
	if err != nil {
		return "", err
	}

	code, ok := perms.Codes[m.RefContact.ID]
	if !ok {
		return "", ErrRecipientDoesNotExist
	}
	query := url.Values{"sharecode": {code}}

	if s.PreviewPath == "" || s.AppSlug == "" {
		path := fmt.Sprintf("/sharings/%s/discovery", s.SID)
		return i.PageURL(path, query), nil
	}

	u := i.SubDomain(s.AppSlug)
	u.Path = s.PreviewPath
	u.RawQuery = query.Encode()
	return u.String(), nil
}
