package mails

import (
	"bytes"
	htmlTemplate "html/template"
	textTemplate "text/template"
)

const (
	// TODOs:
	// Mail templates are directly written as const for now. We could benefit
	// from an asset loader to add them inside the binary from our assets/
	// folder.
	// Also we have connect these templates with our i18n system - using
	// transifex.

	// --- reset_passphrase ---
	mailResetPassHTMLEn = `` +
		`<h1><img src="{{.BaseURL}}assets/images/icon-cozy-mail.png" alt="Cozy Cloud" width="52" height="52" /></h1>

<p>Hello {{.RecipientName}}.<br/> Forgot your password? No worries, let's get you a new one! Click on the link below to safely change it.</p>

<table style="margin: 0 auto;">
<tr>
<td><a href="{{.PassphraseResetLink}}" target="_blank" style="display: block; background: #297EF2; text-transform: uppercase; line-height: 1.1; color: white; text-decoration: none; border-top: 20px solid #297EF2; border-right: 43px solid #297EF2; border-bottom: 20px solid #297EF2; border-left: 43px solid #297EF2; border-radius: 2px;">Reset my password</a></td>
</tr>
</table>

<p>You never asked for a new password? In this case you can forget this email.<br/> Just so you know, you have 15 minutes to choose a new password, then this email will self-destruct.</p>`

	mailResetPassTextEn = `` +
		`Cozy Cloud

Hello {{.RecipientName}}.
Forgot your password? No worries, let's get you a new one! Click on the link below to safely change it.

To reset your password, please go to this URL:
{{.PassphraseResetLink}}

You never asked for a new password? In this case you can forget this email.
Just so you know, you have 15 minutes to choose a new password, then this email will self-destruct.`

	mailResetPassHTMLFr = `` +
		`<h1><img src="{{.BaseURL}}assets/images/icon-cozy-mail.png" alt="Cozy Cloud" width="52" height="52" /></h1>

<p>Bonjour {{.RecipientName}}.<br/> Vous avez oublié votre mot de passe ? Pas de panique, il est temps de vous en trouver un nouveau ! Cliquez sur le lien ci-dessous pour le changer en toute sécurité.</p>

<table style="margin: 0 auto;">
<tr>
<td><a href="{{.PassphraseResetLink}}" target="_blank" style="display: block; background: #297EF2; text-transform: uppercase; line-height: 1.1; color: white; text-decoration: none; border-top: 20px solid #297EF2; border-right: 43px solid #297EF2; border-bottom: 20px solid #297EF2; border-left: 43px solid #297EF2; border-radius: 2px;">Je réinitialise mon mot de passe</a></td>
</tr>
</table>

<p>Vous n'avez jamais demandé de nouveau mot de passe ? Alors vous pouvez ignorer cet email.<br/> Pour information, vous disposez de 15 minutes pour choisir votre nouveau mot de passe, passé ce délai cet email s'auto-détruira.</p>`

	mailResetPassTextFr = `` +
		`Cozy Cloud

Bonjour {{.RecipientName}}.
Vous avez oublié votre mot de passe ? Pas de panique, il est temps de vous en trouver un nouveau ! Cliquez sur le lien ci-dessous pour le changer en toute sécurité.

Pour réinitialiser votre mot de passe, veuillez aller sur l'URL suivante :
{{.PassphraseResetLink}}

Vous n'avez jamais demandé de nouveau mot de passe ? Alors vous pouvez ignorer cet email.
Pour information, vous disposez de 15 minutes pour choisir votre nouveau mot de passe, passé ce délai cet email s'auto-détruira.`

	//  --- sharing_request ---
	mailSharingRequestHTML = `` +
		`<h2>Hey {{.RecipientName}}!</h2>
<p>{{.SharerPublicName}} wants to share a document with you! You will only be able to view it.</p>

<p>The description given is: {{.Description}}.</p>

<table style="margin: 0 auto;">
<tr>
<td><a href="{{.SharingLink}}" target="_blank" style="display: block; background: #297EF2; text-transform: uppercase; line-height: 1.1; color: white; text-decoration: none; border-top: 20px solid #297EF2; border-right: 43px solid #297EF2; border-bottom: 20px solid #297EF2; border-left: 43px solid #297EF2; border-radius: 2px;">Accept this sharing</a></td>
</tr>
</table>`

	mailSharingRequestText = `` +
		`Hey {{.RecipientName}}!
{{.SharerPublicName}} wants to share a document with you! You will only be able to view it.

The description given is: {{.Description}}.

{{.SharingLink}}`

	// --- konnector_error ---
	// TODO: wording and translation of this email is not done
	mailKonnectorErrorHTMLEn = ``
	mailKonnectorErrorTextEn = `` +
		`Hello {{.RecipientName}},

Something wrong happened when we tried to gather the data from your {{.KonnectorName}} account.

If you want more information, please go to the configuration page {{.KonnectorPage}} of your account.

If the problem remains, please contact us at contact@cozycloud.cc

The Cozy Team.`

	mailKonnectorErrorHTMLFr = ``
	mailKonnectorErrorTextFr = `` +
		`Bonjour {{.RecipientName}},

Nous avons rencontré une difficulté pour récupérer vos données provenant de votre compte {{.KonnectorName}}.

Pour plus d'information veuillez vous rendre sur la page: {{.KonnectorPage}} de configuration de votre compte.
Si le problème persiste, n'hésitez pas à nous contacter à contact@cozycloud.cc

L'équipe Cozy.`

	mailArchiveHTMLEn = ``
	mailArchiveTextEn = `` +
		`Hello {{.RecipientName}},

You can now download the archive with all your Cozy data. You can download is by clicking on the following link: {{.Link}}

We wish you a great day,

The Cozy Team`

	mailArchiveHTMLFr = ``
	mailArchiveTextFr = `` +
		`Bonjour {{.RecipientName}},

L'archive contenant l'ensemble des données de votre Cozy est prête à être téléchargée. Vous pouvez la télécharger en cliquant sur ce lien : {{.Link}}
Nous vous souhaitons une très bonne journée,

L'équipe Cozy.`

	mailTwoFactorHTMLFr = ``
	mailTwoFactorTextFr = `` +
		`Bonjour {{.RecipientName}},

Code à utiliser pour l'authentification double-facteurs: {{.TwoFactorPasscode}}
Nous vous souhaitons une très bonne journée,

L'équipe Cozy.`

	mailTwoFactorHTMLEn = ``
	mailTwoFactorTextEn = `` +
		`Hello {{.RecipientName}},

Here is the code to use for two-factor authentication: {{.TwoFactorPasscode}}.
We wish you a great day,

The Cozy Team`
)

// MailTemplate is a struct to define a mail template with HTML and text parts.
type MailTemplate struct {
	Name     string
	BodyHTML string
	BodyText string
}

// MailTemplater contains HTML and text templates to be used with the
// TemplateName field of the MailOptions.
type MailTemplater struct {
	thtml *htmlTemplate.Template
	ttext *textTemplate.Template
}

func newMailTemplater(tmpls []*MailTemplate) *MailTemplater {
	var thtml *htmlTemplate.Template
	var ttext *textTemplate.Template
	var tmpthtml *htmlTemplate.Template
	var tmpttext *textTemplate.Template
	for i, t := range tmpls {
		name := t.Name
		if i == 0 {
			tmpthtml = htmlTemplate.New(name)
			tmpttext = textTemplate.New(name)
			thtml = tmpthtml
			ttext = tmpttext
		} else {
			thtml = tmpthtml.New(name)
			ttext = tmpttext.New(name)
		}
		htmlTemplate.Must(thtml.Parse(t.BodyHTML))
		textTemplate.Must(ttext.Parse(t.BodyText))
	}
	return &MailTemplater{
		thtml: thtml,
		ttext: ttext,
	}
}

// Execute will execute the HTML and text temlates for the template with the
// specified name. It returns the mail parts that should be added to the sent
// mail.
func (m *MailTemplater) Execute(name string, data interface{}) ([]*Part, error) {
	bhtml := new(bytes.Buffer)
	btext := new(bytes.Buffer)
	if err := m.thtml.ExecuteTemplate(bhtml, name, data); err != nil {
		return nil, err
	}
	if err := m.ttext.ExecuteTemplate(btext, name, data); err != nil {
		return nil, err
	}
	return []*Part{
		{Body: btext.String(), Type: "text/plain"},
		{Body: bhtml.String(), Type: "text/html"},
	}, nil
}

func init() {
	mailTemplater = newMailTemplater([]*MailTemplate{
		{
			Name:     "passphrase_reset_en",
			BodyHTML: mailResetPassHTMLEn,
			BodyText: mailResetPassTextEn,
		},
		{
			Name:     "passphrase_reset_fr",
			BodyHTML: mailResetPassHTMLFr,
			BodyText: mailResetPassTextFr,
		},
		{
			Name:     "sharing_request",
			BodyHTML: mailSharingRequestHTML,
			BodyText: mailSharingRequestText,
		},
		{
			Name:     "konnector_error_en",
			BodyHTML: mailKonnectorErrorHTMLEn,
			BodyText: mailKonnectorErrorTextEn,
		},
		{
			Name:     "konnector_error_fr",
			BodyHTML: mailKonnectorErrorHTMLFr,
			BodyText: mailKonnectorErrorTextFr,
		},
		{
			Name:     "archiver_fr",
			BodyHTML: mailArchiveHTMLFr,
			BodyText: mailArchiveTextFr,
		},
		{
			Name:     "archiver_en",
			BodyHTML: mailArchiveHTMLEn,
			BodyText: mailArchiveTextEn,
		},
		{
			Name:     "two_factor_fr",
			BodyHTML: mailTwoFactorHTMLFr,
			BodyText: mailTwoFactorTextFr,
		},
		{
			Name:     "two_factor_en",
			BodyHTML: mailTwoFactorHTMLEn,
			BodyText: mailTwoFactorTextEn,
		},
	})
}
