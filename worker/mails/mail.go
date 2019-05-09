package mails

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/mail"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/gomail"
)

func init() {
	job.AddWorker(&job.WorkerConfig{
		WorkerType:  "sendmail",
		Concurrency: runtime.NumCPU(),
		WorkerFunc:  SendMail,
	})
	initMailTemplates()
}

// var for testability
var mailTemplater MailTemplater
var sendMail = doSendMail

// SendMail is the sendmail worker function.
func SendMail(ctx *job.WorkerContext) error {
	opts := mail.Options{}
	err := ctx.UnmarshalMessage(&opts)
	if err != nil {
		return err
	}
	from := config.GetConfig().NoReplyAddr
	name := config.GetConfig().NoReplyName
	if from == "" {
		from = "noreply@" + utils.StripPort(ctx.Instance.Domain)
	}
	if ctxSettings, err := ctx.Instance.SettingsContext(); err == nil {
		if addr, ok := ctxSettings["noreply_address"].(string); ok && addr != "" {
			from = addr
		}
		if nname, ok := ctxSettings["noreply_name"].(string); ok && nname != "" {
			name = nname
		}
	}
	switch opts.Mode {
	case mail.ModeNoReply:
		toAddr, err := addressFromInstance(ctx.Instance)
		if err != nil {
			return err
		}
		opts.To = []*mail.Address{toAddr}
		opts.From = &mail.Address{Name: name, Email: from}
		opts.RecipientName = toAddr.Name
	case mail.ModeFrom:
		sender, err := addressFromInstance(ctx.Instance)
		if err != nil {
			return err
		}
		name = sender.Name
		opts.ReplyTo = sender
		opts.From = &mail.Address{Name: name, Email: from}
	default:
		return fmt.Errorf("Mail sent with unknown mode %s", opts.Mode)
	}
	if opts.TemplateName != "" && opts.Locale == "" {
		opts.Locale = ctx.Instance.Locale
	}
	return sendMail(ctx, &opts, ctx.Instance.Domain)
}

func addressFromInstance(i *instance.Instance) (*mail.Address, error) {
	doc, err := i.SettingsDocument()
	if err != nil {
		return nil, err
	}
	email, ok := doc.M["email"].(string)
	if !ok {
		return nil, fmt.Errorf("Domain %s has no email in its settings", i.Domain)
	}
	publicName, _ := doc.M["public_name"].(string)
	return &mail.Address{
		Name:  publicName,
		Email: email,
	}, nil
}

func doSendMail(ctx *job.WorkerContext, opts *mail.Options, domain string) error {
	if opts.TemplateName == "" && opts.Subject == "" {
		return errors.New("Missing mail subject")
	}
	if len(opts.To) == 0 {
		return errors.New("Missing mail recipient")
	}
	if opts.From == nil {
		return errors.New("Missing mail sender")
	}
	email := gomail.NewMessage()
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
		toAddresses[i] = email.FormatAddress(to.Email, to.Name)
	}

	var parts []*mail.Part
	var err error

	if opts.TemplateName != "" {
		// Defining the master layout which will wrap the content
		layout := opts.Layout
		if layout == "" {
			layout = mail.DefaultLayout
		}
		opts.Subject, parts, err = RenderMail(ctx, opts.TemplateName, layout, opts.Locale, opts.RecipientName, opts.TemplateValues)
		if err != nil {
			return err
		}
	} else {
		parts = opts.Parts
	}

	headers := map[string][]string{
		"From":    {email.FormatAddress(opts.From.Email, opts.From.Name)},
		"To":      toAddresses,
		"Subject": {opts.Subject},
		"X-Cozy":  {domain},
	}
	if opts.ReplyTo != nil {
		headers["Reply-To"] = []string{
			email.FormatAddress(opts.ReplyTo.Email, opts.ReplyTo.Name),
		}
	}
	email.SetHeaders(headers)
	email.SetDateHeader("Date", date)

	for _, part := range parts {
		if err = addPart(email, part); err != nil {
			return err
		}
	}

	for _, attachment := range opts.Attachments {
		email.Attach(attachment.Filename, gomail.SetCopyFunc(func(w io.Writer) error {
			_, err := w.Write([]byte(attachment.Content))
			return err
		}))
	}

	dialer := gomail.NewDialer(dialerOptions)
	if deadline, ok := ctx.Deadline(); ok {
		dialer.SetDeadline(deadline)
	}
	return dialer.DialAndSend(email)
}

func addPart(mail *gomail.Message, part *mail.Part) error {
	contentType := part.Type
	if contentType != "text/plain" && contentType != "text/html" {
		return fmt.Errorf("Unknown body content-type %s", contentType)
	}
	mail.AddAlternative(contentType, part.Body)
	return nil
}
