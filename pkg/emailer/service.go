package emailer

import (
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
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

// SendEmailCmd contains the informations to send a mail for the instance owner.
type SendEmailCmd struct {
	TemplateName   string
	TemplateValues map[string]interface{}
}

// SendEmail sends a mail to the instance owner.
func (s *EmailerService) SendEmail(inst *instance.Instance, cmd *SendEmailCmd) error {
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
func (s *EmailerService) SendPendingEmail(inst *instance.Instance, cmd *SendEmailCmd) error {
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
