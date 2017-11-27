package instance

import (
	"crypto/sha256"
	"encoding/base32"
	"io"
	"net/http"
	"time"

	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/mssola/user_agent"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/hkdf"
)

var twoFactorTOTPOptions = totp.ValidateOpts{
	Period:    30, // 30s
	Skew:      10, // 30s +- 10*30s = [-5min; 5,5min]
	Digits:    otp.DigitsSix,
	Algorithm: otp.AlgorithmSHA256,
}

// AuthMode defines the authentication mode chosen for the connection to this
// instance.
type AuthMode int

const (
	// Basic authentication mode, only passphrase
	Basic AuthMode = iota
	// TwoFactorMail authentication mode, with passcode sent via email
	TwoFactorMail
)

// AuthModeToString encode authentication mode in a string
func AuthModeToString(authMode AuthMode) string {
	switch authMode {
	case TwoFactorMail:
		return "two_factor_mail"
	default:
		return "basic"
	}
}

// StringToAuthMode converts a string encoded authentication mode into a
// AuthMode int.
func StringToAuthMode(authMode string) AuthMode {
	switch authMode {
	case "two_factor_mail":
		return TwoFactorMail
	default:
		return Basic
	}
}

// GenerateTwoFactorSecrets generates a (token, passcode) pair that can be
// used as a two factor authentication secret value. The token is used to allow
// the two-factor form — meaning the user has correctly entered its passphrase
// and successfully done the first part of the two factor authentication.
//
// The passcode should be send to the user by another mean (mail, SMS, ...)
func (i *Instance) GenerateTwoFactorSecrets() (token []byte, passcode string, err error) {
	// A salt is used when we generate a new 2FA secret to derive a new TOTP
	// function from. This allow us to have TOTP derived from a new key each time
	// we check the first step of the 2FA ("the passphrase step"). This salt is
	// given to the user and signed in the "two-factor-token" MAC.
	salt := crypto.GenerateRandomBytes(sha256.Size)
	token, err = crypto.EncodeAuthMessage(i.totpMACConfig(), salt, nil)
	if err != nil {
		return
	}

	hkdf := hkdf.New(sha256.New, i.SessionSecret, salt, nil)
	key := make([]byte, 32)
	_, err = io.ReadFull(hkdf, key)
	if err != nil {
		return
	}
	passcode, err = totp.GenerateCodeCustom(base32.StdEncoding.EncodeToString(key),
		time.Now().UTC(), twoFactorTOTPOptions)
	return
}

// ValidateTwoFactorPasscode validates the given (token, passcode) pair for two
// factor authentication.
func (i *Instance) ValidateTwoFactorPasscode(token []byte, passcode string) bool {
	salt, err := crypto.DecodeAuthMessage(i.totpMACConfig(), token, nil)
	if err != nil {
		return false
	}

	hkdf := hkdf.New(sha256.New, i.SessionSecret, salt, nil)
	key := make([]byte, 32)
	_, err = io.ReadFull(hkdf, key)
	if err != nil {
		return false
	}
	ok, err := totp.ValidateCustom(passcode, base32.StdEncoding.EncodeToString(key),
		time.Now().UTC(), twoFactorTOTPOptions)
	return ok && err == nil
}

// SendTwoFactorPasscode sends by mail the two factor secret to the owner of
// the instance. It returns the generated token.
func (i *Instance) SendTwoFactorPasscode() ([]byte, error) {
	token, passcode, err := i.GenerateTwoFactorSecrets()
	if err != nil {
		return nil, err
	}
	err = i.SendMail(&Mail{
		TemplateName:   "two_factor",
		TemplateValues: map[string]interface{}{"TwoFactorPasscode": passcode},
	})
	if err != nil {
		return nil, err
	}
	return token, nil
}

// GenerateTwoFactorTrustedDeviceSecret generates a token that can be kept by the
// user on-demand to avoid having two-factor authentication on a specific
// machine.
func (i *Instance) GenerateTwoFactorTrustedDeviceSecret(req *http.Request) ([]byte, error) {
	ua := user_agent.New(req.UserAgent())
	browser, _ := ua.Browser()
	additionalData := []byte(i.Domain + ua.OS() + browser)
	return crypto.EncodeAuthMessage(i.trustedDeviceMACConfig(), nil, additionalData)
}

// ValidateTwoFactorTrustedDeviceSecret validates the given token used to check
// if the computer is trusted to avoid two-factor authorization.
func (i *Instance) ValidateTwoFactorTrustedDeviceSecret(req *http.Request, token []byte) bool {
	ua := user_agent.New(req.UserAgent())
	browser, _ := ua.Browser()
	additionalData := []byte(i.Domain + ua.OS() + browser)
	_, err := crypto.DecodeAuthMessage(i.trustedDeviceMACConfig(), token, additionalData)
	return err == nil
}

func (i *Instance) totpMACConfig() *crypto.MACConfig {
	return &crypto.MACConfig{
		Name:   "totp",
		Key:    i.SessionSecret,
		MaxAge: 0,
		MaxLen: 256,
	}
}

func (i *Instance) trustedDeviceMACConfig() *crypto.MACConfig {
	return &crypto.MACConfig{
		Name:   "trusted-device",
		Key:    i.SessionSecret,
		MaxAge: 0,
		MaxLen: 256,
	}
}
