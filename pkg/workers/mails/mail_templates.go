package mails

import (
	"bytes"
	"fmt"
	"html/template"
	"io/ioutil"
	texttemplate "text/template"

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
func RenderMail(ctx *jobs.WorkerContext, name, layout, locale, recipientName string, templateValues map[string]interface{}) (string, []*Part, error) {
	return mailTemplater.Execute(ctx, name, layout, locale, recipientName, templateValues)
}

// MailTemplater is the list of templates for emails.
// template name -> subject i18n key
type MailTemplater map[string]string

// Execute will execute the HTML and text templates for the template with the
// specified name. It returns the mail parts that should be added to the sent
// mail.
func (m MailTemplater) Execute(ctx *jobs.WorkerContext, name, layout, locale string, recipientName string, data map[string]interface{}) (string, []*Part, error) {
	subjectKey, ok := m[name]
	if !ok {
		err := fmt.Errorf("Could not find email named %q", name)
		return "", nil, err
	}

	subject := i18n.Translate(subjectKey, locale)

	// The subject may contains variables, we are going to ensure it is
	// well-formatted by executing a template on it
	buf := new(bytes.Buffer)
	t, err := template.New("subject").Parse(subject)
	if err != nil {
		return "", nil, err
	}
	if err := t.Execute(buf, data); err != nil {
		return "", nil, err
	}
	subject = buf.String()

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
	if html, err := buildHTML(name, layout, ctx, context, locale, data); err == nil {
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
	funcMap := texttemplate.FuncMap{"t": i18n.Translator(locale)}
	// First templating for translations
	t, err := texttemplate.New("i18n").Funcs(funcMap).Parse(string(b))
	i18nBuf := new(bytes.Buffer)
	err = t.Execute(i18nBuf, data)
	if err != nil {
		return "", err
	}

	t, err = texttemplate.New("text").Funcs(funcMap).Parse(i18nBuf.String())
	if err != nil {
		return "", err
	}
	if err := t.Execute(buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func buildHTML(name string, layout string, ctx *jobs.WorkerContext, context, locale string, data map[string]interface{}) (string, error) {
	buf := new(bytes.Buffer)

	funcMap := template.FuncMap{"t": i18n.Translator(locale)}

	// The following HTML building will be done in several steps. We'll first
	// translate the email content before applying the variables. The same
	// pattern will be done for the layout section.
	// The template package does not seem to be recursive for the underlyings
	// evaluations.

	b, err := loadTemplate("/mails/"+name+".mjml", context)
	if err != nil {
		return "", err
	}

	// Content translated, but no variables evaluated
	content := new(bytes.Buffer)
	t1, err := template.New("content").Funcs(funcMap).Parse(string(b))
	if err != nil {
		return "", err
	}
	err = t1.Execute(content, data)
	if err != nil {
		return "", err
	}

	// Content translated with variables evaluated
	t2, _ := template.New("content").Parse(content.String())
	content.Reset()
	err = t2.Execute(content, data)
	if err != nil {
		return "", err
	}

	tmpTemplate, err := template.New("content").Parse(content.String())

	// Global content
	// Content translated & evaluated
	// Layout translated
	tmpBuf := new(bytes.Buffer)
	b, err = loadTemplate("/mails/"+layout+".mjml", context)
	if err != nil {
		return "", err
	}
	tmpTemplate, err = tmpTemplate.New("layout").Funcs(funcMap).Parse(string(b))
	if err != nil {
		return "", err
	}
	err = tmpTemplate.Execute(tmpBuf, data)
	if err != nil {
		return "", err
	}

	// Eventually execute the HTML mail by evaluating the layout variables
	t, err := template.New("htmlMail").Parse(tmpBuf.String())
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
