package workers

const (
	mailResetPassHTML = `<p>To renew your passphrase, please follow` +
		`<a href="{{.PassphraseResetLink}}">this link</a></p>`

	mailResetPassText = `To renew your passphrase, please go to this URL:` +
		`{{.PassphraseResetLink}}`
)

func init() {
	var err error

	mailTemplater, err = newMailTemplater([]*MailTemplate{
		{
			Name:     "passphrase_reset",
			BodyHTML: mailResetPassHTML,
			BodyText: mailResetPassText,
		},
	})

	if err != nil {
		panic(err)
	}
}
