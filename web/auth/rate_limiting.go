package auth

import (
	"fmt"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/limits"
)

// LoginRateExceeded blocks the instance after too many failed attempts to
// login
func LoginRateExceeded(i *instance.Instance) error {
	err := fmt.Errorf("Instance was blocked because of too many login failed attempts")
	i.Logger().WithField("nspace", "rate_limiting").Warning(err)
	return lifecycle.Block(i, instance.BlockedLoginFailed.Code)
}

// TwoFactorRateExceeded regenerates a new 2FA passcode after too many failed
// attempts to login
func TwoFactorRateExceeded(i *instance.Instance) error {
	if err := limits.CheckRateLimit(i, limits.TwoFactorGenerationType); err != nil {
		return TwoFactorGenerationExceeded(i)
	}
	// Reset the key and send a new passcode to the user
	limits.ResetCounter(i, limits.TwoFactorType)
	_, err := lifecycle.SendTwoFactorPasscode(i)
	return err
}

// TwoFactorGenerationExceeded checks if there was too many attempts to
// regenerate a 2FA code within an hour
func TwoFactorGenerationExceeded(i *instance.Instance) error {
	err := fmt.Errorf("Instance was blocked because of too many 2FA passcode generations")
	i.Logger().WithField("nspace", "rate_limiting").Warning(err)

	return lifecycle.Block(i, instance.BlockedLoginFailed.Code)
}
