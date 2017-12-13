package mails

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/gomail"
)

func init() {
	jobs.AddWorker("sendmail", &jobs.WorkerConfig{
		Concurrency: runtime.NumCPU(),
		WorkerFunc:  SendMail,
	})
}

const (
	// ModeNoReply is the no-reply mode of a mail, to send mail "to" the
	// user's mail, as a noreply@
	ModeNoReply = "noreply"
	// ModeFrom is the "from" mode of a mail, to send mail "from" the user's
	// mail.
	ModeFrom = "from"
)

// var for testability
var mailTemplater *MailTemplater
var sendMail = doSendMail

// Address contains the name and mail of a mail recipient.
type Address struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Attachment is for attaching a file to the mail
type Attachment struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

// Options should be used as the options of a mail with manually defined
// content: body and body content-type. It is used as the input of the
// "sendmail" worker.
type Options struct {
	Mode           string                `json:"mode"`
	From           *Address              `json:"from"`
	To             []*Address            `json:"to"`
	ReplyTo        *Address              `json:"reply_to,omitempty"`
	Subject        string                `json:"subject"`
	Dialer         *gomail.DialerOptions `json:"dialer,omitempty"`
	Date           *time.Time            `json:"date"`
	Parts          []*Part               `json:"parts"`
	RecipientName  string                `json:"recipient_name"`
	TemplateName   string                `json:"template_name"`
	TemplateValues interface{}           `json:"template_values"`
	Attachments    []*Attachment         `json:"attachments,omitempty"`
	Locale         string                `json:"locale"`
	domain         string
}

// Part represent a part of the content of the mail. It has a type
// specifying the content type of the part, and a body.
type Part struct {
	Type string `json:"type"`
	Body string `json:"body"`
}

// SendMail is the sendmail worker function.
func SendMail(ctx *jobs.WorkerContext) error {
	opts := Options{}
	err := ctx.UnmarshalMessage(&opts)
	if err != nil {
		return err
	}
	opts.domain = ctx.Domain()
	from := config.GetConfig().NoReply
	if from == "" {
		from = "noreply@" + utils.StripPort(opts.domain)
	}
	switch opts.Mode {
	case ModeNoReply:
		toAddr, err := addressFromDomain(opts.domain)
		if err != nil {
			return err
		}
		opts.To = []*Address{toAddr}
		opts.From = &Address{Email: from}
		opts.RecipientName = toAddr.Name
	case ModeFrom:
		sender, err := addressFromDomain(opts.domain)
		if err != nil {
			return err
		}
		opts.ReplyTo = sender
		opts.From = &Address{Name: sender.Name, Email: from}
	default:
		return fmt.Errorf("Mail sent with unknown mode %s", opts.Mode)
	}
	if opts.TemplateName != "" && opts.Locale == "" {
		i, err := instance.Get(opts.domain)
		if err != nil {
			return err
		}
		opts.Locale = i.Locale
	}
	return sendMail(ctx, &opts)
}

func addressFromDomain(domain string) (*Address, error) {
	// TODO: cleanup this settings fetching
	db := couchdb.SimpleDatabasePrefix(domain)
	doc := &couchdb.JSONDoc{}
	err := couchdb.GetDoc(db, consts.Settings, consts.InstanceSettingsID, doc)
	if err != nil {
		return nil, err
	}
	email, ok := doc.M["email"].(string)
	if !ok {
		return nil, fmt.Errorf("Domain %s has no email in its settings", domain)
	}
	publicName, _ := doc.M["public_name"].(string)
	return &Address{
		Name:  publicName,
		Email: email,
	}, nil
}

func doSendMail(ctx context.Context, opts *Options) error {
	if opts.TemplateName == "" && opts.Subject == "" {
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

	var parts []*Part
	var err error
	if opts.TemplateName != "" {
		opts.Subject, parts, err = mailTemplater.Execute(opts.TemplateName, opts.Locale, opts.RecipientName, opts.TemplateValues)
		if err != nil {
			return err
		}
	} else {
		parts = opts.Parts
	}

	headers := map[string][]string{
		"From":    {mail.FormatAddress(opts.From.Email, opts.From.Name)},
		"To":      toAddresses,
		"Subject": {opts.Subject},
		"X-Cozy":  {opts.domain},
	}
	if opts.ReplyTo != nil {
		headers["Reply-To"] = []string{
			mail.FormatAddress(opts.ReplyTo.Email, opts.ReplyTo.Name),
		}
	}
	mail.SetHeaders(headers)
	mail.SetDateHeader("Date", date)

	for _, part := range parts {
		if err = addPart(mail, part); err != nil {
			return err
		}
	}

	for _, attachment := range opts.Attachments {
		mail.Attach(attachment.Filename, gomail.SetCopyFunc(func(w io.Writer) error {
			_, err := w.Write([]byte(attachment.Content))
			return err
		}))
	}

	dialer := gomail.NewDialer(dialerOptions)
	if deadline, ok := ctx.Deadline(); ok {
		dialer.SetDeadline(deadline)
	}
	return dialer.DialAndSend(mail)
}

func addPart(mail *gomail.Message, part *Part) error {
	contentType := part.Type
	if contentType != "text/plain" && contentType != "text/html" {
		return fmt.Errorf("Unknown body content-type %s", contentType)
	}
	mail.AddAlternative(contentType, part.Body)
	return nil
}
