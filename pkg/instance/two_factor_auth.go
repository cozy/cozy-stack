package instance

import (
	"encoding/base32"
	"time"

	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/pquerna/otp/totp"
)

// AuthMode defines the authentication mode chosen for the connection to this
// instance.
type AuthMode int

const (
	// Basic authentication mode, only passphrase
	Basic AuthMode = iota
	// TwoFactor authentication mode, with passphrase + passcode sent via another
	// factor.
	TwoFactor
)

// GenerateTwoFactorSecrets generates a (token, passcode) pair that can be
// used as a two factor authentication secret value. The token is used to allow
// the two-factor form — meaning the user has correctly entered its passphrase
// and successfully done the first part of the two factor authentication.
//
// The passcode should be send to the user by another mean (mail, SMS, ...)
func (i *Instance) GenerateTwoFactorSecrets() (token []byte, passcode string, err error) {
	passcode, err = totp.GenerateCodeCustom(base32.StdEncoding.EncodeToString(i.SessionSecret),
		time.Now().UTC(), twoFactorTOTPOptions)
	if err != nil {
		return
	}
	token, err = crypto.EncodeAuthMessage(i.totpMACConfig(), []byte(i.Domain))
	return
}

// ValidateTwoFactorPasscode validates the given (token, passcode) pair for two
// factor authentication.
func (i *Instance) ValidateTwoFactorPasscode(token []byte, passcode string) bool {
	v, err := crypto.DecodeAuthMessage(i.totpMACConfig(), token)
	if err != nil || string(v) != i.Domain {
		return false
	}
	ok, err := totp.ValidateCustom(passcode, base32.StdEncoding.EncodeToString(i.SessionSecret),
		time.Now().UTC(), twoFactorTOTPOptions)
	if err != nil || !ok {
		return false
	}
	return true
}

// SendTwoFactorPasscode sends by mail the two factor secret to the owner of
// the instance. It returns the generated token.
func (i *Instance) SendTwoFactorPasscode() ([]byte, error) {
	token, passcode, err := i.GenerateTwoFactorSecrets()
	if err != nil {
		return nil, err
	}
	err = i.SendMail(&Mail{
		SubjectKey:   "Mail Two factor subject",
		TemplateName: "two_factor",
		TemplateValues: map[string]interface{}{
			"TwoFactorPasscode": passcode,
		},
	})
	if err != nil {
		return nil, err
	}
	return token, nil
}

func (i *Instance) totpMACConfig() *crypto.MACConfig {
	return &crypto.MACConfig{
		Name:   "totp",
		Key:    i.SessionSecret,
		MaxAge: int64(twoFactorTOTPOptions.Period * twoFactorTOTPOptions.Skew),
		MaxLen: 256,
	}
}
