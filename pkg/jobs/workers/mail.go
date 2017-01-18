package workers

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/gomail"
)

func init() {
	jobs.AddWorker("sendmail", &jobs.WorkerConfig{
		Concurrency:  4,
		MaxExecCount: 3,
		Timeout:      10 * time.Second,
		WorkerFunc:   SendMail,
	})

	jobs.AddWorker("sendmail-template", &jobs.WorkerConfig{
		Concurrency:  4,
		MaxExecCount: 3,
		Timeout:      10 * time.Second,
		WorkerFunc:   SendMailTemplate,
	})
}

// MailAddress contains the name and mail of a mail recipient.
type MailAddress struct {
	Name string `json:"name"`
	Mail string `json:"mail"`
}

type mailOptions struct {
	From    *MailAddress          `json:"from"`
	To      []*MailAddress        `json:"to"`
	Subject string                `json:"subject"`
	Dialer  *gomail.DialerOptions `json:"dialer,omitempty"`
	Date    *time.Time            `json:"date"`
}

// MailManualOptions should be used as the options of a mail with manually
// defined content: body and body content-type. It is used as the input of the
// "sendmail" worker.
type MailManualOptions struct {
	*mailOptions
	Body     string `json:"body"`
	BodyType string `json:"body_type"`
}

// MailTemplateOptions should be used as the options of a mail with a templated
// content: template and values. It is used as the input of the
// "sendmail-template" worker.
type MailTemplateOptions struct {
	*mailOptions
	Template string      `json:"template"`
	Values   interface{} `json:"values"`
}

// SendMail is the sendmail worker function.
func SendMail(ctx context.Context, m *jobs.Message) error {
	var opts MailManualOptions
	if err := m.Unmarshal(&opts); err != nil {
		return err
	}
	return sendMail(ctx, opts.mailOptions, opts.BodyType, opts.Body)
}

// SendMailTemplate is the sendmail-template worker function.
func SendMailTemplate(ctx context.Context, m *jobs.Message) error {
	var opts MailTemplateOptions
	if err := m.Unmarshal(&opts); err != nil {
		return err
	}
	t, err := template.New("mail").Parse(opts.Template)
	if err != nil {
		return err
	}
	b := new(bytes.Buffer)
	if err = t.Execute(b, opts.Values); err != nil {
		return err
	}
	return sendMail(ctx, opts.mailOptions, "text/html", b.String())
}

func sendMail(ctx context.Context, opts *mailOptions, bodyType, body string) error {
	if bodyType == "" {
		bodyType = "text/plain"
	}
	if bodyType != "text/plain" && bodyType != "text/html" {
		return fmt.Errorf("Unknown body content-type %s", bodyType)
	}
	if opts.Subject == "" {
		return fmt.Errorf("Missing mail subject")
	}

	dialerOptions := opts.Dialer
	if dialerOptions == nil {
		dialerOptions = config.GetConfig().Mail
	}

	mail := gomail.NewMessage()
	toAddresses := make([]string, len(opts.To))
	for i, to := range opts.To {
		toAddresses[i] = mail.FormatAddress(to.Mail, to.Name)
	}

	mail.SetBody(bodyType, body)
	mail.SetHeaders(map[string][]string{
		"From":    {mail.FormatAddress(opts.From.Mail, opts.From.Name)},
		"To":      toAddresses,
		"Subject": {opts.Subject},
	})
	if opts.Date != nil {
		mail.SetDateHeader("Date", *opts.Date)
	}

	dialer := gomail.NewDialer(dialerOptions)
	if deadline, ok := ctx.Deadline(); ok {
		dialer.SetDeadline(deadline)
	}
	return dialer.DialAndSend(mail)
}
