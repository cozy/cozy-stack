package emailer

import (
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
)

var service *EmailerService

// Emailer allows to send a pre-formatted email to an instance owner.
//
// This interface has several implementations:
// - [EmailerService] sending email via an async job
// - [Mock] with a mock implementation
type Emailer interface {
	SendEmail(inst *instance.Instance, cmd *SendEmailCmd) error
}

// Init the emailer package by setting up a service based on the
// global config and setup the global functions.
func Init() *EmailerService {
	service = NewEmailerService(job.System())

	return service
}

// SendEmail send a mail to the instance owner.
//
// Deprecated: use [EmailerService.SendEmail] instead.
func SendEmail(inst *instance.Instance, cmd *SendEmailCmd) error {
	return service.SendEmail(inst, cmd)
}
