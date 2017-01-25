package workers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
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
	Name  string `json:"name"`
	Email string `json:"email"`
}

// MailOptions should be used as the options of a mail with manually defined
// content: body and body content-type. It is used as the input of the
// "sendmail" worker.
type MailOptions struct {
	Mode    string                `json:"mode"`
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
	opts := &MailOptions{}
	err := m.Unmarshal(&opts)
	if err != nil {
		return err
	}
	domain := ctx.Value(jobs.ContextDomainKey).(string)
	switch opts.Mode {
	case "noreply":
		toAddr, err := addressFromDomain(domain)
		if err != nil {
			return err
		}
		opts.To = []*MailAddress{toAddr}
		opts.From = &MailAddress{Email: "noreply@" + domain}
	case "from":
		fromAddr, err := addressFromDomain(domain)
		if err != nil {
			return err
		}
		opts.From = fromAddr
	default:
		return fmt.Errorf("Mail sent with unknown mode %s", opts.Mode)
	}
	return sendMail(ctx, opts)
}

func addressFromDomain(domain string) (*MailAddress, error) {
	in, err := instance.Get(domain)
	if err != nil {
		return nil, err
	}
	doc := &couchdb.JSONDoc{}
	err = couchdb.GetDoc(in, consts.Settings, consts.InstanceSettingsID, doc)
	if err != nil {
		return nil, err
	}
	email, ok := doc.M["email"].(string)
	if !ok {
		return nil, fmt.Errorf("Domain %s has no email in its settings", domain)
	}
	return &MailAddress{
		Name:  "", // TODO: no name settings for an instance ?
		Email: email,
	}, nil
}

func doSendMail(ctx context.Context, opts *MailOptions) error {
	if opts.Subject == "" {
		return errors.New("Missing mail subject")
	}
	if len(opts.To) == 0 {
		return errors.New("Missing mail recipient")
	}
	if opts.From == nil {
		return errors.New("Missing mail sender")
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
		toAddresses[i] = mail.FormatAddress(to.Email, to.Name)
	}
	mail.SetHeaders(map[string][]string{
		"From":    {mail.FormatAddress(opts.From.Email, opts.From.Name)},
		"To":      toAddresses,
		"Subject": {opts.Subject},
	})
	mail.SetDateHeader("Date", date)
	for _, part := range opts.Parts {
		if err := addPart(mail, part); err != nil {
			return err
		}
	}
	dialer := gomail.NewDialer(dialerOptions)
	if deadline, ok := ctx.Deadline(); ok {
		dialer.SetDeadline(deadline)
	}
	return dialer.DialAndSend(mail)
}

func addPart(mail *gomail.Message, part *MailPart) error {
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

// var for testability
var sendMail = doSendMail
