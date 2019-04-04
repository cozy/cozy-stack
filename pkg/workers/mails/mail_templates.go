package mails

import (
	"bytes"
	"fmt"
	"html/template"
	"io/ioutil"

	"github.com/cozy/cozy-stack/pkg/i18n"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/statik/fs"
)

// MailTemplate is a struct to define a mail template with HTML and text parts.
type MailTemplate struct {
	Name    string
	Subject string
	Intro   string
	Outro   string
	Actions []MailAction
	Entries []MailEntry
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
	mailTemplater = MailTemplater{
		"passphrase_reset":             "Mail Reset Passphrase Subject",
		"archiver":                     "Mail Archive Subject",
		"two_factor":                   "Mail Two Factor Subject",
		"two_factor_mail_confirmation": "Mail Two Factor Mail Confirmation Subject",
		"new_connection":               "Mail New Connection Subject",
		"new_registration":             "Mail New Registration Subject",
		"sharing_request":              "Mail Sharing Request Subject",
		"notifications_diskquota":      "Notifications Disk Quota Subject",
	}
}

// RenderMail returns a rendered mail for the given template name with the
// specified locale, recipient name and template data values.
func RenderMail(ctx *jobs.WorkerContext, name, locale, recipientName string, templateValues map[string]interface{}) (string, []*Part, error) {
	return mailTemplater.Execute(ctx, name, locale, recipientName, templateValues)
}

// MailTemplater is the list of templates for emails.
// template name -> subject i18n key
type MailTemplater map[string]string

// Execute will execute the HTML and text templates for the template with the
// specified name. It returns the mail parts that should be added to the sent
// mail.
func (m MailTemplater) Execute(ctx *jobs.WorkerContext, name, locale string, recipientName string, data map[string]interface{}) (string, []*Part, error) {
	subjectKey, ok := m[name]
	if !ok {
		err := fmt.Errorf("Could not find email named %q", name)
		return "", nil, err
	}

	subject := i18n.Translate(subjectKey, locale)
	context := ctx.Instance.ContextName
	data["Locale"] = locale

	text, err := buildText(name, context, locale, data)
	if err != nil {
		return "", nil, err
	}
	parts := []*Part{
		{Body: text, Type: "text/plain"},
	}

	// If we can generate the HTML, we should still send the mail with the text
	// part.
	if html, err := buildHTML(name, ctx, context, locale, data); err == nil {
		parts = append(parts, &Part{Body: html, Type: "text/html"})
	} else {
		ctx.Logger().Errorf("Cannot generate HTML mail: %s", err)
	}
	return subject, parts, nil
}

func buildText(name, context, locale string, data map[string]interface{}) (string, error) {
	buf := new(bytes.Buffer)
	b, err := loadTemplate("/mails/"+name+".text", context)
	if err != nil {
		return "", err
	}
	funcMap := template.FuncMap{"t": i18n.Translator(locale)}
	t, err := template.New("text").Funcs(funcMap).Parse(string(b))
	if err != nil {
		return "", err
	}
	if err := t.Execute(buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func buildHTML(name string, ctx *jobs.WorkerContext, context, locale string, data map[string]interface{}) (string, error) {
	buf := new(bytes.Buffer)
	b, err := loadTemplate("/mails/"+name+".mjml", context)
	if err != nil {
		return "", err
	}
	funcMap := template.FuncMap{"t": i18n.Translator(locale)}
	t, err := template.New("content").Funcs(funcMap).Parse(string(b))
	if err != nil {
		return "", err
	}
	b, err = loadTemplate("/mails/layout.mjml", context)
	if err != nil {
		return "", err
	}
	t, err = t.New("layout").Funcs(funcMap).Parse(string(b))
	if err != nil {
		return "", err
	}
	if err := t.Execute(buf, data); err != nil {
		return "", err
	}
	html, err := execMjml(ctx, buf.Bytes())
	if err != nil {
		return "", err
	}
	return string(html), nil
}

func loadTemplate(name, context string) ([]byte, error) {
	var f *bytes.Reader
	if context != "" {
		f, _ = fs.Open(name, context)
	}
	if f == nil {
		var err error
		f, err = fs.Open(name) // Default context
		if err != nil {
			return nil, err
		}
	}
	return ioutil.ReadAll(f)
}
