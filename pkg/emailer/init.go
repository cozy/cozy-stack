package emailer

import (
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
)

var service *EmailerService

// Init the emailer package by setting up a service based on the
// global config and setup the global functions.
func Init() *EmailerService {
	service = NewEmailerService(job.System())

	return service
}

// SendEmail send a mail to the instance owner.
//
// Deprecated: use [EmailerService.SendMail] instead.
func SendEmail(inst *instance.Instance, cmd *SendEmailCmd) error {
	return service.SendEmail(inst, cmd)
}
