package token

import (
	"time"

	"github.com/cozy/cozy-stack/pkg/prefixer"
)

// Service used to generate token to protect and authenticate an operation
// outside cozy.
//
// This service is used to protect unauthenticated operation done oustide the
// stack. For example when we need to validate an email with an URL link.
// Without this random token inside the url anyone could validate the email for
// anyone as user come unauthenticated.
type Service interface {
	GenerateAndSave(db prefixer.Prefixer, op Operation, resource string, lifetime time.Duration) (string, error)
	Validate(db prefixer.Prefixer, op Operation, resource, token string) error
}
