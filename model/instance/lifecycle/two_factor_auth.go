package lifecycle

import "github.com/cozy/cozy-stack/model/instance"

// SendTwoFactorPasscode sends by mail the two factor secret to the owner of
// the instance. It returns the generated token.
func SendTwoFactorPasscode(inst *instance.Instance) ([]byte, error) {
	token, passcode, err := inst.GenerateTwoFactorSecrets()
	if err != nil {
		return nil, err
	}
	err = SendMail(inst, &Mail{
		TemplateName:   "two_factor",
		TemplateValues: map[string]interface{}{"TwoFactorPasscode": passcode},
	})
	if err != nil {
		return nil, err
	}
	return token, nil
}

// SendMailConfirmationCode send a code to validate the email of the instance
// in order to activate 2FA.
func SendMailConfirmationCode(inst *instance.Instance) error {
	passcode, err := inst.GenerateMailConfirmationCode()
	if err != nil {
		return err
	}
	return SendMail(inst, &Mail{
		TemplateName:   "two_factor_mail_confirmation",
		TemplateValues: map[string]interface{}{"TwoFactorActivationPasscode": passcode},
	})
}
