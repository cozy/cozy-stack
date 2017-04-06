package workers

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

<a href="{{.PassphraseResetLink}}" style="color:white; text-decoration:none; text-transform:uppercase; font-weight: bold;">
<table cellspacing="0" cellpadding="0" style="background-color:#297EF2; border-radius: 3px;">
<tr><td colspan="3">&nbsp;</td></tr>
<tr><td width="25">&nbsp;</td><td style="color:white;">Reset my password</td><td width="25">&nbsp;</td></tr>
<tr><td colspan="3">&nbsp;</td></tr>
</table>
</a>

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

<a href="{{.PassphraseResetLink}}" style="color:white; text-decoration:none; text-transform:uppercase; font-weight: bold;">
<table cellspacing="0" cellpadding="0" style="background-color:#297EF2; border-radius: 3px;">
<tr><td colspan="3">&nbsp;</td></tr>
<tr><td width="25">&nbsp;</td><td style="color:white;">Je réinitialise mon mot de passe</td><td width="25">&nbsp;</td></tr>
<tr><td colspan="3">&nbsp;</td></tr>
</table>
</a>

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

<form action="{{.OAuthQueryString}}">
	<input type="submit" value="Accept this sharing" />
</form>
</p>`

	mailSharingRequestText = `` +
		`Hey {{.RecipientName}}!
{{.SharerPublicName}} wants to share a document with you! You will only be able to view it.

The description given is: {{.Description}}.

{{.OAuthQueryString}}`
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
func (m *MailTemplater) Execute(name string, data interface{}) ([]*MailPart, error) {
	bhtml := new(bytes.Buffer)
	btext := new(bytes.Buffer)
	if err := m.thtml.ExecuteTemplate(bhtml, name, data); err != nil {
		return nil, err
	}
	if err := m.ttext.ExecuteTemplate(btext, name, data); err != nil {
		return nil, err
	}
	return []*MailPart{
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
	})
}
