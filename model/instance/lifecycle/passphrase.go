package lifecycle

import (
	"crypto/subtle"
	"encoding/hex"
	"net/url"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/crypto"
)

func registerPassphrase(inst *instance.Instance, pass, tok []byte) error {
	if len(pass) == 0 {
		return instance.ErrMissingPassphrase
	}
	if len(inst.RegisterToken) == 0 {
		return instance.ErrMissingToken
	}
	if subtle.ConstantTimeCompare(inst.RegisterToken, tok) != 1 {
		return instance.ErrInvalidToken
	}
	hash, err := crypto.GenerateFromPassphrase(pass)
	if err != nil {
		return err
	}
	inst.RegisterToken = nil
	setPassphraseAndSecret(inst, hash)
	return nil
}

// RegisterPassphrase replace the instance registerToken by a passphrase
func RegisterPassphrase(inst *instance.Instance, pass, tok []byte) error {
	if err := registerPassphrase(inst, pass, tok); err != nil {
		return err
	}
	return update(inst)
}

// RequestPassphraseReset generates a new registration token for the user to
// renew its password.
func RequestPassphraseReset(inst *instance.Instance) error {
	// If a registration token is set, we do not generate another token than the
	// registration one, and bail.
	if inst.RegisterToken != nil {
		inst.Logger().Info("Passphrase reset ignored: not registered")
		return nil
	}
	// If a passphrase reset token is set and valid, we do not generate new one,
	// and bail.
	if inst.PassphraseResetToken != nil && inst.PassphraseResetTime != nil &&
		time.Now().UTC().Before(*inst.PassphraseResetTime) {
		inst.Logger().Infof("Passphrase reset ignored: already sent at %s",
			inst.PassphraseResetTime.String())
		return instance.ErrResetAlreadyRequested
	}
	resetTime := time.Now().UTC().Add(config.PasswordResetInterval())
	inst.PassphraseResetToken = crypto.GenerateRandomBytes(instance.PasswordResetTokenLen)
	inst.PassphraseResetTime = &resetTime
	if err := update(inst); err != nil {
		return err
	}
	// Send a mail containing the reset url for the user to actually reset its
	// passphrase.
	resetURL := inst.PageURL("/auth/passphrase_renew", url.Values{
		"token": {hex.EncodeToString(inst.PassphraseResetToken)},
	})
	publicName, err := inst.PublicName()
	if err != nil {
		return err
	}
	return SendMail(inst, &Mail{
		TemplateName: "passphrase_reset",
		TemplateValues: map[string]interface{}{
			"BaseURL":             inst.PageURL("/", nil),
			"PassphraseResetLink": resetURL,
			"PublicName":          publicName,
		},
	})
}

// Mail contains the informations to send a mail for the instance owner.
type Mail struct {
	TemplateName   string
	TemplateValues map[string]interface{}
}

// SendMail send a mail to the instance owner.
func SendMail(inst *instance.Instance, m *Mail) error {
	msg, err := job.NewMessage(map[string]interface{}{
		"mode":            "noreply",
		"template_name":   m.TemplateName,
		"template_values": m.TemplateValues,
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

// CheckPassphraseRenewToken checks whether the given token is good to use for
// resetting the passphrase.
func CheckPassphraseRenewToken(inst *instance.Instance, tok []byte) error {
	if inst.PassphraseResetToken == nil {
		return instance.ErrMissingToken
	}
	if inst.PassphraseResetTime != nil && !time.Now().UTC().Before(*inst.PassphraseResetTime) {
		return instance.ErrMissingToken
	}
	if subtle.ConstantTimeCompare(inst.PassphraseResetToken, tok) != 1 {
		return instance.ErrInvalidToken
	}
	return nil
}

// PassphraseRenew changes the passphrase to the specified one if the given
// token matches the `PassphraseResetToken` field.
func PassphraseRenew(inst *instance.Instance, pass, tok []byte) error {
	err := CheckPassphraseRenewToken(inst, tok)
	if err != nil {
		return err
	}
	hash, err := crypto.GenerateFromPassphrase(pass)
	if err != nil {
		return err
	}
	inst.PassphraseResetToken = nil
	inst.PassphraseResetTime = nil
	setPassphraseAndSecret(inst, hash)
	return update(inst)
}

// UpdatePassphrase replace the passphrase
func UpdatePassphrase(inst *instance.Instance, pass, current []byte, twoFactorPasscode string, twoFactorToken []byte) error {
	if len(pass) == 0 {
		return instance.ErrMissingPassphrase
	}
	// With two factor authentication, we do not check the validity of the
	// current passphrase, but the validity of the pair passcode/token which has
	// been exchanged against the current passphrase.
	if inst.HasAuthMode(instance.TwoFactorMail) {
		if !inst.ValidateTwoFactorPasscode(twoFactorToken, twoFactorPasscode) {
			return instance.ErrInvalidTwoFactor
		}
	} else {
		// the needUpdate flag is not checked against since the passphrase will be
		// regenerated with updated parameters just after, if the passphrase match.
		_, err := crypto.CompareHashAndPassphrase(inst.PassphraseHash, current)
		if err != nil {
			return instance.ErrInvalidPassphrase
		}
	}
	hash, err := crypto.GenerateFromPassphrase(pass)
	if err != nil {
		return err
	}
	setPassphraseAndSecret(inst, hash)
	return update(inst)
}

// ForceUpdatePassphrase replace the passphrase without checking the current one
func ForceUpdatePassphrase(inst *instance.Instance, newPassword []byte) error {
	if len(newPassword) == 0 {
		return instance.ErrMissingPassphrase
	}

	hash, err := crypto.GenerateFromPassphrase(newPassword)
	if err != nil {
		return err
	}
	setPassphraseAndSecret(inst, hash)
	return update(inst)
}

func setPassphraseAndSecret(inst *instance.Instance, hash []byte) {
	inst.PassphraseHash = hash
	inst.SessionSecret = crypto.GenerateRandomBytes(instance.SessionSecretLen)
}

// CheckPassphrase confirm an instance passport
func CheckPassphrase(inst *instance.Instance, pass []byte) error {
	if len(pass) == 0 {
		return instance.ErrMissingPassphrase
	}

	needUpdate, err := crypto.CompareHashAndPassphrase(inst.PassphraseHash, pass)
	if err != nil {
		return err
	}

	if !needUpdate {
		return nil
	}

	newHash, err := crypto.GenerateFromPassphrase(pass)
	if err != nil {
		return err
	}

	inst.PassphraseHash = newHash
	if err = update(inst); err != nil {
		inst.Logger().Error("Failed to update hash in db", err)
	}
	return nil
}
