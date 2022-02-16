package oauth

import (
	"crypto/sha256"
	"encoding/base32"
	"io"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/hkdf"
)

var macConfig = crypto.MACConfig{
	Name:   "confirm-flagship",
	MaxAge: 0,
	MaxLen: 256,
}

var totpOptions = totp.ValidateOpts{
	Period:    30, // 30s
	Skew:      10, // 30s +- 10*30s = [-5min; 5,5min]
	Digits:    otp.DigitsSix,
	Algorithm: otp.AlgorithmSHA256,
}

// SendConfirmFlagshipCode sends by mail a code to the owner of the instance.
// It returns the generated token which can be used to check the code.
func SendConfirmFlagshipCode(inst *instance.Instance, clientID string) ([]byte, error) {
	token, code, err := GenerateConfirmCode(inst, clientID)
	if err != nil {
		return nil, err
	}

	publicName, _ := inst.PublicName()
	msg, err := job.NewMessage(map[string]interface{}{
		"mode":          "noreply",
		"template_name": "confirm_flagship",
		"template_values": map[string]interface{}{
			"Code":       code,
			"PublicName": publicName,
		},
	})
	if err != nil {
		return nil, err
	}
	_, err = job.System().PushJob(inst, &job.JobRequest{
		WorkerType: "sendmail",
		Message:    msg,
	})
	if err != nil {
		return nil, err
	}
	return token, nil
}

// CheckFlagshipCode returns true if the code is correct and can be used to set
// the flagship flag on the given OAuth client.
func CheckFlagshipCode(inst *instance.Instance, clientID string, token []byte, code string) bool {
	salt, err := crypto.DecodeAuthMessage(macConfig, inst.SessionSecret(), token, nil)
	if err != nil {
		return false
	}

	input := []byte(clientID + "-" + string(inst.SessionSecret()))
	h := hkdf.New(sha256.New, input, salt, nil)
	key := make([]byte, 32)
	_, err = io.ReadFull(h, key)
	if err != nil {
		return false
	}
	secret := base32.StdEncoding.EncodeToString(key)

	ok, err := totp.ValidateCustom(code, secret, time.Now().UTC(), totpOptions)
	return ok && err == nil
}

// GenerateConfirmCode generate a 6-digits code and the token to check it. They
// can be used to manually confirm that an OAuth client is the flagship app.
func GenerateConfirmCode(inst *instance.Instance, clientID string) ([]byte, string, error) {
	salt := crypto.GenerateRandomBytes(sha256.Size)
	token, err := crypto.EncodeAuthMessage(macConfig, inst.SessionSecret(), salt, nil)
	if err != nil {
		return nil, "", err
	}

	input := []byte(clientID + "-" + string(inst.SessionSecret()))
	h := hkdf.New(sha256.New, input, salt, nil)
	key := make([]byte, 32)
	_, err = io.ReadFull(h, key)
	if err != nil {
		return nil, "", err
	}
	secret := base32.StdEncoding.EncodeToString(key)

	code, err := totp.GenerateCodeCustom(secret, time.Now().UTC(), totpOptions)
	return token, code, err
}
