package mails

import "errors"

// MailTemplate is a struct to define a mail template with HTML and text parts.
type MailTemplate struct {
	Name       string
	NoGreeting bool
	Subject    string
	Intro      string
	Outro      string
	Actions    []MailAction
	Entries    []MailEntry
}

// MailAction describes an action button in a mail template.
type MailAction struct {
	Instructions string
	Text         string
	Link         string
}

// MailEntry describes an row entry in a mail template.
type MailEntry struct {
	Key string
	Val string
}

func initMailTemplates() {
	mailTemplater = &MailTemplater{[]*MailTemplate{
		{
			Name:    "passphrase_reset",
			Subject: "Mail Reset Passphrase Subject",
			Intro:   "Mail Reset Passphrase Intro",
			Actions: []MailAction{
				{
					Instructions: "Mail Reset Passphrase Button instruction",
					Text:         "Mail Reset Passphrase Button text",
					Link:         "{{.PassphraseResetLink}}",
				},
			},
			Outro: "Mail Reset Passphrase Outro",
		},
		{
			Name:    "archiver",
			Subject: "Mail Archive Subject",
			Intro:   "Mail Archive Intro",
			Actions: []MailAction{
				{
					Instructions: "Mail Archive Button instruction",
					Text:         "Mail Archive Button text",
					Link:         "{{.ArchiveLink}}",
				},
			},
		},
		{
			Name:    "two_factor",
			Subject: "Mail Two Factor Subject",
			Intro:   "Mail Two Factor Intro",
			Outro:   "Mail Two Factor Outro",
		},
		{
			Name:    "two_factor_mail_confirmation",
			Subject: "Mail Two Factor Mail Confirmation Subject",
			Intro:   "Mail Two Factor Mail Confirmation Intro",
			Outro:   "Mail Two Factor Mail Confirmation Outro",
		},
		{
			Name:    "new_connexion",
			Subject: "Mail New Connection Subject",
			Intro:   "Mail New Connection Intro",
			Entries: []MailEntry{
				{Key: "Mail New Connection Place", Val: "{{.City}}, {{.Country}}"},
				{Key: "Mail New Connection IP", Val: "{{.IP}}"},
				{Key: "Mail New Connection Browser", Val: "{{.Browser}}"},
				{Key: "Mail New Connection OS", Val: "{{.OS}}"},
			},
			Actions: []MailAction{
				{
					Instructions: "Mail New Connection Change Passphrase instruction",
					Text:         "Mail New Connection Change Passphrase text",
					Link:         "{{.ChangePassphraseLink}}",
				},
			},
			Outro: "Mail New Connection Outro",
		},
		{
			Name:    "new_registration",
			Subject: "Mail New Registration Subject",
			Intro:   "Mail New Registration Intro",
			Actions: []MailAction{
				{
					Instructions: "Mail New Registration Devices instruction",
					Text:         "Mail New Registration Devices text",
					Link:         "{{.DevicesLink}}",
				},
				// {
				//  Instructions: "Mail New Registration Revoke instruction",
				//  Text:         "Mail New Registration Revoke text",
				//  Link:         "{{.RevokeLink}}",
				// },
			},
		},
		{
			Name:    "sharing_request",
			Subject: "Mail Sharing Request Subject",
			Intro:   "Mail Sharing Request Intro",
			Actions: []MailAction{
				{
					Instructions: "Mail Sharing Request Button instruction",
					Text:         "Mail Sharing Request Button text",
					Link:         "{{.SharingLink}}",
				},
			},
		},

		// Notifications
		{
			Name:    "notifications_diskquota",
			Subject: "Notifications Disk Quota Subject",
			Intro:   "Notifications Disk Quota Intro",
			Actions: []MailAction{
				{
					Instructions: "Notifications Disk Quota offers instruction",
					Text:         "Notifications Disk Quota offers text",
					Link:         "{{.OffersLink}}",
				},
				{
					Instructions: "Notifications Disk Quota free instructions",
					Text:         "Notifications Disk Quota free text",
					Link:         "{{.CozyDriveLink}}",
				},
			},
		},
	}}
}

// RenderMail returns a rendered mail for the given template name with the
// specified locale, recipient name and template data values.
func RenderMail(name, locale, recipientName string, templateValues interface{}) (string, []*Part, error) {
	return mailTemplater.Execute(name, locale, recipientName, templateValues)
}

// MailTemplater contains HTML and text templates to be used with the
// TemplateName field of the MailOptions.
type MailTemplater struct {
	tmpls []*MailTemplate
}

// Execute will execute the HTML and text templates for the template with the
// specified name. It returns the mail parts that should be added to the sent
// mail.
func (m *MailTemplater) Execute(name, locale string, recipientName string, data interface{}) (subject string, parts []*Part, err error) {
	err = errors.New("Not implemented")
	return
}
