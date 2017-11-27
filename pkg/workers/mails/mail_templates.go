package mails

import (
	"bytes"
	"fmt"
	"sync"
	"text/template"

	"github.com/cozy/cozy-stack/pkg/i18n"
	"github.com/matcornic/hermes"
)

// TODO: avoid having a lock by init-ing hermes instances after loading
// locales.
var hermeses map[string]hermes.Hermes
var hermesesMu sync.Mutex

func getHermes(locale string) hermes.Hermes {
	hermesesMu.Lock()
	defer hermesesMu.Unlock()
	if hermeses == nil {
		hermeses = make(map[string]hermes.Hermes, len(i18n.SupportedLocales))
		for _, locale := range i18n.SupportedLocales {
			hermeses[locale] = hermes.Hermes{
				Theme:         new(hermes.Default),
				TextDirection: hermes.TDLeftToRight,
				Product: hermes.Product{
					Name:        "Cozy",
					Link:        "https://cozy.io",
					Logo:        "https://files.cozycloud.cc/mailing/logo-cozy-notif-mail_2x.png",
					Copyright:   "--",
					TroubleText: i18n.Translate("Mail Trouble Text", locale),
				},
			}
		}
	}
	h, ok := hermeses[locale]
	if !ok {
		h = hermeses[i18n.DefaultLocale]
	}
	return h
}

// MailTemplate is a struct to define a mail template with HTML and text parts.
type MailTemplate struct {
	Name     string
	Greeting bool
	Subject  string
	Intro    string
	Outro    string
	Actions  []MailAction

	cacheMu sync.Mutex
	cache   map[string]*mailCache
}

// MailAction describes an action button in a mail template.
type MailAction struct {
	Instructions string
	Text         string
	Link         string
}

type mailCache struct {
	subject  string
	greeting string
	intro    *template.Template
	outro    *template.Template
	actions  []mailActionCache
}

type mailActionCache struct {
	instructions string
	text         string
	link         *template.Template
}

func (m *mailCache) ToBody(recipientName string, data interface{}) (body hermes.Body, err error) {
	var intros []string
	var outros []string
	introB := new(bytes.Buffer)
	outroB := new(bytes.Buffer)
	if err = m.intro.Execute(introB, data); err != nil {
		return
	}
	if err = m.outro.Execute(outroB, data); err != nil {
		return
	}
	for _, b := range bytes.Split(introB.Bytes(), []byte("\n")) {
		if len(b) > 0 {
			intros = append(intros, string(b))
		}
	}
	for _, b := range bytes.Split(outroB.Bytes(), []byte("\n")) {
		if len(b) > 0 {
			outros = append(outros, string(b))
		}
	}
	as := make([]hermes.Action, len(m.actions))
	for i, a := range m.actions {
		link := new(bytes.Buffer)
		if err = a.link.Execute(link, data); err != nil {
			return
		}
		as[i] = hermes.Action{
			Instructions: a.instructions,
			Button: hermes.Button{
				Text: a.text,
				Link: link.String(),
			},
		}
	}
	body.Greeting = m.greeting
	body.Name = recipientName
	body.Intros = intros
	body.Outros = outros
	body.Actions = as
	return
}

// MailTemplater contains HTML and text templates to be used with the
// TemplateName field of the MailOptions.
type MailTemplater struct {
	tmpls []*MailTemplate
}

// Execute will execute the HTML and text temlates for the template with the
// specified name. It returns the mail parts that should be added to the sent
// mail.
func (m *MailTemplater) Execute(name, locale string, recipientName string, data interface{}) (subject string, parts []*Part, err error) {
	var tpl *MailTemplate
	for _, t := range m.tmpls {
		if name == t.Name {
			tpl = t
			break
		}
	}
	if tpl == nil {
		err = fmt.Errorf("Could not find email named %q", name)
		return
	}

	var c *mailCache
	{
		tpl.cacheMu.Lock()

		if tpl.cache == nil {
			tpl.cache = make(map[string]*mailCache)
		}

		var ok bool
		c, ok = tpl.cache[locale]
		if !ok {
			c = new(mailCache)
			if tpl.Greeting {
				c.greeting = i18n.Translate("Mail Greeting", locale)
			}
			c.subject = i18n.Translate(tpl.Subject, locale)
			c.intro, err = createTemplate(tpl.Intro, locale)
			if err != nil {
				return
			}
			c.outro, err = createTemplate(tpl.Outro, locale)
			if err != nil {
				return
			}
			c.actions = make([]mailActionCache, len(tpl.Actions))
			for i, a := range tpl.Actions {
				c.actions[i].instructions = i18n.Translate(a.Instructions, locale)
				c.actions[i].text = i18n.Translate(a.Text, locale)
				c.actions[i].link, err = template.New("").Parse(a.Link)
				if err != nil {
					return
				}
			}
			tpl.cache[locale] = c
		}

		tpl.cacheMu.Unlock()
	}

	body, err := c.ToBody(recipientName, data)
	if err != nil {
		return
	}

	h := getHermes(locale)
	email := hermes.Email{Body: body}
	html, err := h.GenerateHTML(email)
	if err != nil {
		return
	}
	text, err := h.GeneratePlainText(email)
	if err != nil {
		return
	}

	subject = c.subject
	parts = []*Part{
		{Body: html, Type: "text/html"},
		{Body: text, Type: "text/plain"},
	}
	return
}

func createTemplate(s string, locale string) (*template.Template, error) {
	return template.New("").Parse(i18n.Translate(s, locale))
}

func init() {
	mailTemplater = &MailTemplater{[]*MailTemplate{
		{
			Name:    "passphrase_reset",
			Subject: "Mail Reset Passphrase Subject",
			Intro:   "Mail Reset Passphrase Intro",
			Actions: []MailAction{
				{
					Instructions: "Mail Reset Passphrase Button instruction",
					Text:         "Mail Reset Passphrase Button text",
					Link:         "{{.PassphraseResetLink}}",
				},
			},
			Outro: "Mail Reset Passphrase Outro",
		},
		{
			Name:    "sharing_request",
			Subject: "Mail Sharing Request Subject",
			Intro:   "Mail Sharing Request Intro",
			Actions: []MailAction{
				{
					Instructions: "Mail Sharing Request Button instruction",
					Text:         "Mail Sharing Request Button text",
					Link:         "{{.SharingLink}}",
				},
			},
		},
		{
			Name:    "konnector_error",
			Subject: "Mail Konnector Error Subject",
			Intro:   "Mail Konnector Error Intro",
		},
		{
			Name:    "archiver",
			Subject: "Mail Archive Subject",
			Intro:   "Mail Archive Intro",
			Actions: []MailAction{
				{
					Instructions: "Mail Archive Button instruction",
					Text:         "Mail Archive Button text",
					Link:         "{{.ArchiveLink}}",
				},
			},
		},
		{
			Name:    "two_factor",
			Subject: "Mail Two Factor Subject",
			Intro:   "Mail Two Factor Intro",
		},
	}}
}
