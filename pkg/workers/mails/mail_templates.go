package mails

import (
	"bytes"
	"fmt"
	"html/template"
	"io/ioutil"
	text "text/template"

	"github.com/cozy/cozy-stack/pkg/i18n"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/statik/fs"
)

func initMailTemplates() {
	mailTemplater = MailTemplater{
		"passphrase_reset":             subjectEntry{"Mail Reset Passphrase Subject", nil},
		"archiver":                     subjectEntry{"Mail Archive Subject", nil},
		"two_factor":                   subjectEntry{"Mail Two Factor Subject", nil},
		"two_factor_mail_confirmation": subjectEntry{"Mail Two Factor Mail Confirmation Subject", nil},
		"new_connection":               subjectEntry{"Mail New Connection Subject", nil},
		"new_registration":             subjectEntry{"Mail New Registration Subject", nil},
		"sharing_request":              subjectEntry{"Mail Sharing Request Subject", []string{"SharerPublicName"}},
		"notifications_diskquota":      subjectEntry{"Notifications Disk Quota Subject", nil},
	}
}

// RenderMail returns a rendered mail for the given template name with the
// specified locale, recipient name and template data values.
func RenderMail(ctx *jobs.WorkerContext, name, layout, locale, recipientName string, templateValues map[string]interface{}) (string, []*Part, error) {
	return mailTemplater.Execute(ctx, name, layout, locale, recipientName, templateValues)
}

// MailTemplater is the list of templates for emails.
type MailTemplater map[string]subjectEntry

// subjectEntry is a i18n key for the subject message, and some optional
// variable names.
type subjectEntry struct {
	Key       string
	Variables []string
}

// Execute will execute the HTML and text templates for the template with the
// specified name. It returns the mail parts that should be added to the sent
// mail.
func (m MailTemplater) Execute(ctx *jobs.WorkerContext, name, layout, locale string, recipientName string, data map[string]interface{}) (string, []*Part, error) {
	entry, ok := m[name]
	if !ok {
		err := fmt.Errorf("Could not find email named %q", name)
		return "", nil, err
	}

	var vars []interface{}
	for _, name := range entry.Variables {
		vars = append(vars, data[name])
	}

	subject := i18n.Translate(entry.Key, locale, vars...)
	context := ctx.Instance.ContextName
	data["Locale"] = locale
	if ctx.Instance != nil {
		data["InstanceURL"] = ctx.Instance.PageURL("/", nil)
	}

	txt, err := buildText(name, context, locale, data)
	if err != nil {
		return "", nil, err
	}
	parts := []*Part{
		{Body: txt, Type: "text/plain"},
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
	funcMap := text.FuncMap{"t": i18n.Translator(locale)}
	t, err := text.New("text").Funcs(funcMap).Parse(string(b))
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
	b, err := loadTemplate("/mails/"+name+".mjml", context)
	if err != nil {
		return "", err
	}
	funcMap := template.FuncMap{"t": i18n.Translator(locale)}
	t, err := template.New("content").Funcs(funcMap).Parse(string(b))
	if err != nil {
		return "", err
	}
	b, err = loadTemplate("/mails/"+layout+".mjml", context)
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
