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

// The sharing-dependant information: the recipient's name, the sharer's public
// name, the description of the sharing, and the OAuth query string.
type mailTemplateValues struct {
	RecipientName    string
	SharerPublicName string
	Description      string
	SharingLink      string
}

// SendMails send an email to each recipient, so that they can accept or
// refuse the sharing.
func SendMails(instance *instance.Instance, s *Sharing) error {
	sharerPublicName, err := instance.PublicName()
	if err != nil || sharerPublicName == "" {
		sharerPublicName = "Someone"
	}
	// Fill in the description.
	desc := s.Description
	if desc == "" {
		desc = "Surprise!"
	}

	for _, recipient := range s.Recipients {
		if recipient.Status != consts.SharingStatusPending &&
			recipient.Status != consts.SharingStatusMailNotSent {
			continue
		}

		// TODO we should check that the contact is not nil
		mailAddress, erro := recipient.Contact(instance).ToMailAddress()
		if erro != nil {
			instance.Logger().Errorf("[sharing] Recipient has no email address: %#v", recipient.RefContact)
			err = ErrRecipientHasNoEmail
			recipient.Status = consts.SharingStatusMailNotSent
			if erru := couchdb.UpdateDoc(instance, s); erru != nil {
				instance.Logger().Errorf("[sharing] Can't save status: %s", err)
			}
			continue
		}
		link := linkForRecipient(instance, s, &recipient)
		mailValues := &mailTemplateValues{
			RecipientName:    mailAddress.Name,
			SharerPublicName: sharerPublicName,
			Description:      desc,
			SharingLink:      link,
		}
		msg, erro := jobs.NewMessage(mails.Options{
			Mode:           "from",
			To:             []*mails.Address{mailAddress},
			TemplateName:   "sharing_request",
			TemplateValues: mailValues,
			RecipientName:  mailValues.RecipientName,
		})
		if erro == nil {
			_, erro = globals.GetBroker().PushJob(&jobs.JobRequest{
				Domain:     instance.Domain,
				WorkerType: "sendmail",
				Options:    nil,
				Message:    msg,
			})
		}
		if erro != nil {
			instance.Logger().Errorf("[sharing] Can't send email: %s", erro)
			err = ErrMailCouldNotBeSent
			recipient.Status = consts.SharingStatusMailNotSent
			if erru := couchdb.UpdateDoc(instance, s); erru != nil {
				instance.Logger().Errorf("[sharing] Can't save status: %s", err)
			}
		}
	}

	return err
}

func linkForRecipient(i *instance.Instance, s *Sharing, rs *Member) string {
	code := "XXX" // TODO
	query := url.Values{"sharecode": {code}}

	if s.PreviewPath == "" || s.AppSlug == "" {
		path := fmt.Sprintf("/sharings/%s/discovery", s.SID)
		return i.PageURL(path, query)
	}

	u := i.SubDomain(s.AppSlug)
	u.Path = s.PreviewPath
	u.RawQuery = query.Encode()
	return u.String()
}
