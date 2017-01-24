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
}

// MailAddress contains the name and mail of a mail recipient.
type MailAddress struct {
	Name string `json:"name"`
	Mail string `json:"mail"`
}

// MailOptions should be used as the options of a mail with manually defined
// content: body and body content-type. It is used as the input of the
// "sendmail" worker.
type MailOptions struct {
	From    *MailAddress          `json:"from"`
	To      []*MailAddress        `json:"to"`
	Subject string                `json:"subject"`
	Dialer  *gomail.DialerOptions `json:"dialer,omitempty"`
	Date    *time.Time            `json:"date"`
	Parts   []*MailPart           `json:"parts"`
}

// MailPart represent a part of the content of the mail. It has a type
// specifying the content type of the part, and can used with:
//   - Template field as a html template parsed and executed with the specified
//     Values field
//   - Body field used directly as body of the part.
type MailPart struct {
	Type     string      `json:"body_type"`
	Body     string      `json:"body"`
	Template string      `json:"template"`
	Values   interface{} `json:"values"`
}

// SendMail is the sendmail worker function.
func SendMail(ctx context.Context, m *jobs.Message) error {
	var opts MailOptions
	if err := m.Unmarshal(&opts); err != nil {
		return err
	}
	if opts.Subject == "" {
		return fmt.Errorf("Missing mail subject")
	}
	mail := gomail.NewMessage()
	dialerOptions := opts.Dialer
	if dialerOptions == nil {
		dialerOptions = config.GetConfig().Mail
	}
	var date time.Time
	if opts.Date == nil {
		date = time.Now()
	} else {
		date = *opts.Date
	}
	toAddresses := make([]string, len(opts.To))
	for i, to := range opts.To {
		toAddresses[i] = mail.FormatAddress(to.Mail, to.Name)
	}
	mail.SetHeaders(map[string][]string{
		"From":    {mail.FormatAddress(opts.From.Mail, opts.From.Name)},
		"To":      toAddresses,
		"Subject": {opts.Subject},
	})
	mail.SetDateHeader("Date", date)
	for _, part := range opts.Parts {
		if err := addContent(mail, part); err != nil {
			return err
		}
	}
	dialer := gomail.NewDialer(dialerOptions)
	if deadline, ok := ctx.Deadline(); ok {
		dialer.SetDeadline(deadline)
	}
	return dialer.DialAndSend(mail)
}

func addContent(mail *gomail.Message, part *MailPart) error {
	contentType := part.Type
	var body string
	if part.Template != "" {
		contentType = "text/html"
		t, err := template.New("mail").Parse(part.Template)
		if err != nil {
			return err
		}
		b := new(bytes.Buffer)
		if err = t.Execute(b, part.Values); err != nil {
			return err
		}
		body = b.String()
	} else {
		body = part.Body
	}
	if contentType == "" {
		contentType = "text/plain"
	}
	if contentType != "text/plain" && contentType != "text/html" {
		return fmt.Errorf("Unknown body content-type %s", contentType)
	}
	mail.AddAlternative(contentType, body)
	return nil
}
