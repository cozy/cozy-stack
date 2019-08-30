package lifecycle

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/crypto"
)

// PassParameters are the parameters for setting a new passphrase
type PassParameters struct {
	Pass       []byte // Pass is the password hashed on client side, but not yet on server.
	Iterations int    // Iterations is the number of iterations applied by PBKDF2 on client side.
	Key        string // Key is the encryption key (encrypted, and in CipherString format).
}

func registerPassphrase(inst *instance.Instance, tok []byte, params PassParameters) error {
	if len(params.Pass) == 0 {
		return instance.ErrMissingPassphrase
	}
	if len(inst.RegisterToken) == 0 {
		return instance.ErrMissingToken
	}
	if subtle.ConstantTimeCompare(inst.RegisterToken, tok) != 1 {
		return instance.ErrInvalidToken
	}
	if params.Iterations == 0 {
		pass, masterKey, iterations := emulateClientSideHashing(inst, params.Pass)
		params.Pass, params.Iterations = pass, iterations
		if params.Key == "" {
			if err := CreatePassphraseKey(inst, masterKey); err != nil {
				return err
			}
		}
	}
	hash, err := crypto.GenerateFromPassphrase(params.Pass)
	if err != nil {
		return err
	}
	inst.RegisterToken = nil
	setPassphraseKdfAndSecret(inst, hash, params.Iterations)
	return nil
}

// RegisterPassphrase replace the instance registerToken by a passphrase
func RegisterPassphrase(inst *instance.Instance, tok []byte, params PassParameters) error {
	if err := registerPassphrase(inst, tok, params); err != nil {
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
func PassphraseRenew(inst *instance.Instance, tok []byte, params PassParameters) error {
	err := CheckPassphraseRenewToken(inst, tok)
	if err != nil {
		return err
	}
	if params.Iterations == 0 {
		pass, masterKey, iterations := emulateClientSideHashing(inst, params.Pass)
		params.Pass, params.Iterations = pass, iterations
		if params.Key == "" {
			if err := CreatePassphraseKey(inst, masterKey); err != nil {
				return err
			}
		}
	}
	hash, err := crypto.GenerateFromPassphrase(params.Pass)
	if err != nil {
		return err
	}
	inst.PassphraseResetToken = nil
	inst.PassphraseResetTime = nil
	setPassphraseKdfAndSecret(inst, hash, params.Iterations)
	return update(inst)
}

// UpdatePassphrase replace the passphrase
func UpdatePassphrase(inst *instance.Instance, pass, current []byte, twoFactorPasscode string, twoFactorToken []byte, kdfIterations int) error {
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
	setPassphraseKdfAndSecret(inst, hash, kdfIterations)
	return update(inst)
}

// ForceUpdatePassphrase replace the passphrase without checking the current one
func ForceUpdatePassphrase(inst *instance.Instance, newPassword []byte) error {
	if len(newPassword) == 0 {
		return instance.ErrMissingPassphrase
	}

	pass, masterKey, kdfIterations := emulateClientSideHashing(inst, newPassword)
	if err := CreatePassphraseKey(inst, masterKey); err != nil {
		return err
	}
	hash, err := crypto.GenerateFromPassphrase(pass)
	if err != nil {
		return err
	}
	setPassphraseKdfAndSecret(inst, hash, kdfIterations)
	return update(inst)
}

func emulateClientSideHashing(inst *instance.Instance, password []byte) ([]byte, []byte, int) {
	kdfIterations := crypto.DefaultPBKDF2Iterations
	salt := inst.PassphraseSalt()
	hashed, masterKey := crypto.HashPassWithPBKDF2(password, salt, kdfIterations)
	return hashed, masterKey, kdfIterations
}

func setPassphraseKdfAndSecret(inst *instance.Instance, hash []byte, kdfIterations int) {
	inst.PassphraseHash = hash
	inst.PassphraseKdf = instance.PBKDF2_SHA256
	inst.PassphraseKdfIterations = kdfIterations
	inst.SessionSecret = crypto.GenerateRandomBytes(instance.SessionSecretLen)
}

// CreatePassphraseKey creates an encryption key for Bitwarden, and keeps an
// encrypted version of it in the instance (the key is the master key, derived
// from the master password).
// See https://github.com/jcs/rubywarden/blob/master/API.md
func CreatePassphraseKey(inst *instance.Instance, masterKey []byte) error {
	pt := crypto.GenerateRandomBytes(64)
	iv := crypto.GenerateRandomBytes(16)

	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return err
	}
	dst := make([]byte, len(pt))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(dst, pt)
	iv64 := base64.StdEncoding.EncodeToString(iv)
	dst64 := base64.StdEncoding.EncodeToString(dst)

	// 0 means AES-256-CBC
	inst.PassphraseKey = "0." + iv64 + "|" + dst64
	return nil
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
