package emailer

import (
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/mail"
)

// EmailerService allows to send emails.
//
// This broker send the emails via anrasynchronous job.
type EmailerService struct {
	jobBroker job.Broker
}

// NewEmailerService instantiates an [EmailerService].
func NewEmailerService(jobBroker job.Broker) *EmailerService {
	return &EmailerService{jobBroker}
}

// TransactionalEmailCmd contains the information to send a transactional email
// to the instance owner.
type TransactionalEmailCmd struct {
	TemplateName   string
	TemplateValues map[string]interface{}
}

// SendEmail sends a mail to the instance owner.
func (s *EmailerService) SendEmail(inst *instance.Instance, cmd *TransactionalEmailCmd) error {
	if cmd.TemplateName == "" || cmd.TemplateValues == nil {
		return ErrMissingTemplate
	}

	msg, err := job.NewMessage(map[string]interface{}{
		"mode":            "noreply",
		"template_name":   cmd.TemplateName,
		"template_values": cmd.TemplateValues,
	})
	if err != nil {
		return err
	}

	_, err = s.jobBroker.PushJob(inst, &job.JobRequest{
		WorkerType: "sendmail",
		Message:    msg,
	})

	return err
}

// SendPendingEmail sends a mail to the instance owner on their new pending
// email address. It is used to confirm that they can receive emails on the new
// email address.
func (s *EmailerService) SendPendingEmail(inst *instance.Instance, cmd *TransactionalEmailCmd) error {
	if cmd.TemplateName == "" || cmd.TemplateValues == nil {
		return ErrMissingTemplate
	}

	msg, err := job.NewMessage(map[string]interface{}{
		"mode":            "pending",
		"template_name":   cmd.TemplateName,
		"template_values": cmd.TemplateValues,
	})
	if err != nil {
		return err
	}

	_, err = s.jobBroker.PushJob(inst, &job.JobRequest{
		WorkerType: "sendmail",
		Message:    msg,
	})

	return err
}

// CampaignEmailCmd contains the information required to send a campaign email
// to the instance owner.
type CampaignEmailCmd struct {
	Parts   []mail.Part
	Subject string
}

// SendCampaignEmail sends a campaign email to the instance owner with the
// given cmd content via the dedicated campaign mail server.
func (s *EmailerService) SendCampaignEmail(inst *instance.Instance, cmd *CampaignEmailCmd) error {
	if cmd.Subject == "" {
		return ErrMissingSubject
	}
	if cmd.Parts == nil {
		return ErrMissingContent
	}

	msg, err := job.NewMessage(map[string]interface{}{
		"mode":    mail.ModeCampaign,
		"subject": cmd.Subject,
		"parts":   cmd.Parts,
	})
	if err != nil {
		return err
	}

	_, err = s.jobBroker.PushJob(inst, &job.JobRequest{
		WorkerType: "sendmail",
		Message:    msg,
	})

	return err
}
