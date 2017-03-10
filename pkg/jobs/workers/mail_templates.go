package workers

import (
	"bytes"
	htmlTemplate "html/template"
	textTemplate "text/template"
)

const (
	// TODOs:
	// Mail templates are directly written as const for now. We could benefit
	// from and asset loader to add them inside the binary from our assets/
	// folder.
	// Also we have connect these templates with our i18n system - using
	// transifex.

	// --- reset_passphrase ---
	mailResetPassHTML = `` +
		`<p>To renew your passphrase, please follow
<a href="{{.PassphraseResetLink}}">this link</a></p>`

	mailResetPassText = `` +
		`To renew your passphrase, please go to this URL:
{{.PassphraseResetLink}}`

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
			Name:     "passphrase_reset",
			BodyHTML: mailResetPassHTML,
			BodyText: mailResetPassText,
		},
		{
			Name:     "sharing_request",
			BodyHTML: mailSharingRequestHTML,
			BodyText: mailSharingRequestText,
		},
	})
}
