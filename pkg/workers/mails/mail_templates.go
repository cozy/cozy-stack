package mails

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"text/template"

	"github.com/cozy/cozy-stack/pkg/i18n"
	"github.com/cozy/hermes"
)

// TODO: avoid having a lock by init-ing hermes instances after loading
// locales.
var hermeses map[string]hermes.Hermes
var hermesesMu sync.Mutex

var templateFuncsMap = map[string]interface{}{
	"splitList": func(sep, orig string) []string {
		return strings.Split(orig, sep)
	},
	"replace": func(old, new, src string) string {
		return strings.Replace(src, old, new, -1)
	},
}

func getHermes(locale string) hermes.Hermes {
	hermesesMu.Lock()
	defer hermesesMu.Unlock()
	if hermeses == nil {
		hermeses = make(map[string]hermes.Hermes, len(i18n.SupportedLocales))
		for _, locale := range i18n.SupportedLocales {
			hermeses[locale] = hermes.Hermes{
				Theme:         new(MailTheme),
				TextDirection: hermes.TDLeftToRight,
				Product: hermes.Product{
					Name:        i18n.Translate("Mail Cozy Team", locale),
					Link:        "https://cozy.io",
					Logo:        "https://files.cozycloud.cc/mailing/logo-cozy-notif-mail_2x.png",
					Copyright:   "",
					TroubleText: i18n.Translate("Mail Trouble Text", locale),
				},
				TemplateFuncsMap: templateFuncsMap,
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
	Name       string
	NoGreeting bool
	Subject    string
	Intro      string
	Outro      string
	Actions    []MailAction
	Entries    []MailEntry

	cacheMu sync.Mutex
	cache   map[string]*mailCache
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

type mailCache struct {
	subject   string
	greeting  string
	signature string
	intro     *template.Template
	outro     *template.Template
	actions   []mailActionCache
	entries   []mailEntryCache
}

type mailActionCache struct {
	instructions *template.Template
	text         string
	link         *template.Template
}

type mailEntryCache struct {
	key string
	val *template.Template
}

func (m *mailCache) ToBody(recipientName string, data interface{}) (body hermes.Body, err error) {
	var intros []string
	var outros []string
	introB := new(bytes.Buffer)
	outroB := new(bytes.Buffer)
	if m.intro != nil {
		if err = m.intro.Execute(introB, data); err != nil {
			return
		}
		for _, b := range bytes.Split(introB.Bytes(), []byte("\n")) {
			if len(b) > 0 {
				intros = append(intros, string(b))
			}
		}
	}
	if m.outro != nil {
		if err = m.outro.Execute(outroB, data); err != nil {
			return
		}
		for _, b := range bytes.Split(outroB.Bytes(), []byte("\n")) {
			if len(b) > 0 {
				outros = append(outros, string(b))
			}
		}
	}
	as := make([]hermes.Action, len(m.actions))
	es := make([]hermes.Entry, len(m.entries))
	for i, a := range m.actions {
		link := new(bytes.Buffer)
		if err = a.link.Execute(link, data); err != nil {
			return
		}
		inst := new(bytes.Buffer)
		if err = a.instructions.Execute(inst, data); err != nil {
			return
		}
		as[i] = hermes.Action{
			Instructions: inst.String(),
			Button: hermes.Button{
				Text: a.text,
				Link: link.String(),
			},
		}
	}
	for i, e := range m.entries {
		value := new(bytes.Buffer)
		if err = e.val.Execute(value, data); err != nil {
			return
		}
		es[i] = hermes.Entry{
			Key:   e.key,
			Value: value.String(),
		}
	}
	body.Greeting = m.greeting
	body.Signature = m.signature
	body.Name = recipientName
	body.Intros = intros
	body.Outros = outros
	body.Actions = as
	body.Dictionary = es
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
		defer tpl.cacheMu.Unlock()

		if tpl.cache == nil {
			tpl.cache = make(map[string]*mailCache)
		}

		var ok bool
		c, ok = tpl.cache[locale]
		if !ok {
			c = new(mailCache)
			if !tpl.NoGreeting {
				c.greeting = i18n.Translate("Mail Greeting", locale)
			}
			c.signature = i18n.Translate("Mail Signature", locale)
			if tpl.Subject != "" {
				c.subject = i18n.Translate(tpl.Subject, locale)
			}
			if tpl.Intro != "" {
				c.intro, err = template.New("").Parse(i18n.Translate(tpl.Intro, locale))
				if err != nil {
					return
				}
			}
			if tpl.Outro != "" {
				c.outro, err = template.New("").Parse(i18n.Translate(tpl.Outro, locale))
				if err != nil {
					return
				}
			}
			c.actions = make([]mailActionCache, len(tpl.Actions))
			for i, a := range tpl.Actions {
				if a.Instructions != "" {
					c.actions[i].instructions, err = template.New("").Parse(
						i18n.Translate(a.Instructions, locale))
					if err != nil {
						return
					}
				}
				if a.Text != "" {
					c.actions[i].text = i18n.Translate(a.Text, locale)
				}
				c.actions[i].link, err = template.New("").Parse(a.Link)
				if err != nil {
					return
				}
			}
			c.entries = make([]mailEntryCache, len(tpl.Entries))
			for i, e := range tpl.Entries {
				c.entries[i].key = i18n.Translate(e.Key, locale)
				c.entries[i].val, err = template.New("").Parse(e.Val)
				if err != nil {
					return
				}
			}
			tpl.cache[locale] = c
		}
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

// RenderMail returns a rendered mail for the given template name with the
// specified locale, recipient name and template data values.
func RenderMail(name, locale, recipientName string, templateValues interface{}) (string, []*Part, error) {
	return mailTemplater.Execute(name, locale, recipientName, templateValues)
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
			Outro:   "Mail Two Factor Outro",
		},
		{
			Name:    "two_factor_mail_confirmation",
			Subject: "Mail Two Factor Mail Confirmation Subject",
			Intro:   "Mail Two Factor Mail Confirmation Intro",
			Outro:   "Mail Two Factor Mail Confirmation Outro",
		},
		{
			Name:    "new_connexion",
			Subject: "Mail New Connection Subject",
			Intro:   "Mail New Connection Intro",
			Entries: []MailEntry{
				{Key: "Mail New Connection Place", Val: "{{.City}}, {{.Country}}"},
				{Key: "Mail New Connection IP", Val: "{{.IP}}"},
				{Key: "Mail New Connection Browser", Val: "{{.Browser}}"},
				{Key: "Mail New Connection OS", Val: "{{.OS}}"},
			},
			Actions: []MailAction{
				{
					Instructions: "Mail New Connection Change Passphrase instruction",
					Text:         "Mail New Connection Change Passphrase text",
					Link:         "{{.ChangePassphraseLink}}",
				},
			},
			Outro: "Mail New Connection Outro",
		},
		{
			Name:    "new_registration",
			Subject: "Mail New Registration Subject",
			Intro:   "Mail New Registration Intro",
			Actions: []MailAction{
				{
					Instructions: "Mail New Registration Devices instruction",
					Text:         "Mail New Registration Devices text",
					Link:         "{{.DevicesLink}}",
				},
				// {
				//  Instructions: "Mail New Registration Revoke instruction",
				//  Text:         "Mail New Registration Revoke text",
				//  Link:         "{{.RevokeLink}}",
				// },
			},
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

		// Notifications
		{
			Name:    "notifications_diskquota",
			Subject: "Notifications Disk Quota Subject",
			Intro:   "Notifications Disk Quota Intro",
			Actions: []MailAction{
				{
					Instructions: "Notifications Disk Quota offers instruction",
					Text:         "Notifications Disk Quota offers text",
					Link:         "{{.OffersLink}}",
				},
				{
					Instructions: "Notifications Disk Quota free instructions",
					Text:         "Notifications Disk Quota free text",
					Link:         "{{.CozyDriveLink}}",
				},
			},
		},
	}}
}

// MailTheme is the theme by default
type MailTheme struct{}

// Name returns the name of the theme
func (dt *MailTheme) Name() string {
	return "cozy"
}

// HTMLTemplate returns a Golang template that will generate an HTML email.
func (dt *MailTheme) HTMLTemplate() string {
	return `
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <meta http-equiv="Content-Type" content="text/html; charset=UTF-8" />
  <style type="text/css" rel="stylesheet" media="all">
    *:not(br):not(tr):not(html) {
      font-family: Arial, 'Helvetica Neue', Helvetica, sans-serif;
      -webkit-box-sizing: border-box;
      box-sizing: border-box;
    }
    body {
      width: 100% !important;
      height: 100%;
      margin: 0;
      line-height: 1.4;
      background-color: #F5F6F7;
      color: #74787E;
      -webkit-text-size-adjust: none;
    }
    a {
      color: #3869D4;
    }

    .email-wrapper {
      width: 100%;
      margin: 0;
      padding: 0;
      background-color: #F5F6F7;
    }
    .email-content {
      width: 100%;
      margin: 0;
      padding: 0;
    }

    .email-masthead {
      padding: 16px 0;
      text-align: center;
    }
    .email-masthead_logo {
      max-width: 400px;
      border: 0;
    }
    .email-masthead_name {
      font-size: 16px;
      font-weight: bold;
      color: #2F3133;
      text-decoration: none;
      text-shadow: 0 1px 0 white;
    }
    .email-logo {
      max-height: 50px;
    }

    .email-body {
      width: 100%;
      margin: 0;
      padding: 0;
    }
    .email-body_inner {
      width: 600px;
      margin: 0 auto;
      padding: 0;
      background-color: #FFF;
      border-bottom: 1px solid #95999D;
      border-left: 1px solid #E2E4E9;
      border-right: 1px solid #E2E4E9;
    }
    .email-footer {
      width: 570px;
      margin: 0 auto;
      text-align: center;
    }
    .email-footer p {
      color: #95999D;
    }
    .body-action {
      width: 100%;
      margin: 14px auto;
      padding: 0;
      text-align: center;
    }
    .body-dictionary {
      width: 100%;
      overflow: hidden;
      margin: 20px auto 10px;
      padding: 0;
    }
    .body-dictionary dd {
      margin: 0 0 10px 0;
    }
    .body-dictionary dt {
      clear: both;
      color: #000;
      font-weight: bold;
    }
    .body-dictionary dd {
      margin-left: 0;
      margin-bottom: 10px;
    }
    .body-sub {
      text-align: center;
      font-weight: bold;
      padding: 8px 0;
    }
    .body-sub td {
      padding: 0;
    }
    .body-sub p {
      margin: 0;
    }
    .body-sub a {
      word-break: break-all;
      color: #297EF1;
      text-decoration: none;
    }
    .content-cell {
      padding: 24px;
    }
    .content-block {
      border-top: 1px solid #95999D;
      border-bottom: 1px solid #95999D;
      margin-bottom: 24px;
      padding: 24px 0 8px;
      text-align: center;
    }
    .align-right {
      text-align: right;
    }

    h1 {
      margin: 0 0 24px;
      color: #32363F;
      font-size: 20px;
      font-weight: bold;
      line-height: 1.2;
      text-align: center;
    }
    h2 {
      margin-top: 0;
      color: #2F3133;
      font-size: 16px;
      font-weight: bold;
    }
    h3 {
      margin-top: 0;
      color: #2F3133;
      font-size: 14px;
      font-weight: bold;
    }
    blockquote {
      margin: 1.7rem 0;
      padding-left: 0.85rem;
      border-left: 10px solid #F0F2F4;
    }
    blockquote p {
        font-size: 1.1rem;
        color: #999;
    }
    blockquote cite {
        display: block;
        text-align: right;
        color: #666;
        font-size: 1.2rem;
    }
    cite {
      display: block;
      font-size: 0.925rem;
    }
    cite:before {
      content: "\2014 \0020";
    }
    p {
      margin-top: 0;
      color: #32363F;
      font-size: 16px;
      line-height: 1.5em;
    }
    p.sub {
      font-size: 12px;
    }
    p.center {
      text-align: center;
    }
    table {
      width: 100%;
    }
    th {
      padding: 0px 5px;
      padding-bottom: 8px;
      border-bottom: 1px solid #EDEFF2;
    }
    th p {
      margin: 0;
      color: #9BA2AB;
      font-size: 12px;
    }
    td {
      padding: 10px 5px;
      color: #74787E;
      font-size: 15px;
      line-height: 18px;
    }
    .content {
      align: center;
      padding: 0;
    }

    .data-wrapper {
      width: 100%;
      margin: 0;
      padding: 35px 0;
    }
    .data-table {
      width: 100%;
      margin: 0;
    }
    .data-table th {
      text-align: left;
      padding: 0px 5px;
      padding-bottom: 8px;
      border-bottom: 1px solid #EDEFF2;
    }
    .data-table th p {
      margin: 0;
      color: #9BA2AB;
      font-size: 12px;
    }
    .data-table td {
      padding: 10px 5px;
      color: #74787E;
      font-size: 15px;
      line-height: 18px;
    }

    .button {
      display: inline-block;
      margin-bottom: 8px;
      width: 300px;
      background-color: #297EF1;
      border-radius: 3px;
      color: #ffffff;
      font-size: 15px;
      line-height: 40px;
      text-align: center;
      text-decoration: none;
      text-transform: uppercase;
      -webkit-text-size-adjust: none;
      mso-hide: all;
    }

    .button-fallback {
      font-size: 12px;
    }

    .button-fallback a {
      font-weight: bold;
      color: #32363F;
      text-decoration: none;
      word-break: break-all;
    }

    .footer-link {
      font-size: 12px;
      letter-spacing: 0.6px;
      text-align: right;
      color: #95999d;
      text-decoration: none;
    }

    .footer-link:hover,
    .footer-link:focus {
      text-decoration: underline;
    }

    @media only screen and (max-width: 600px) {
      .email-body_inner,
      .email-footer {
        width: 100% !important;
      }
    }
    @media only screen and (max-width: 500px) {
      .button {
        width: 100% !important;
      }
    }
  </style>
</head>
<body dir="{{.Hermes.TextDirection}}">
  <table class="email-wrapper" width="100%" cellpadding="0" cellspacing="0">
    <tr>
      <td class="content">
        <table class="email-content" width="100%" cellpadding="0" cellspacing="0">
          <!-- Logo -->
          <tr>
            <td class="email-masthead">
              <a class="email-masthead_name" href="{{.Hermes.Product.Link}}" target="_blank">
                {{ if .Hermes.Product.Logo }}
                  <img src="{{.Hermes.Product.Logo | url }}" class="email-logo" />
                {{ else }}
                  {{ .Hermes.Product.Name }}
                {{ end }}
                </a>
            </td>
          </tr>
          <!-- Email Body -->
          <tr>
            <td class="email-body" width="100%">
              <table class="email-body_inner" align="center" width="570" cellpadding="0" cellspacing="0">
                <!-- Body content -->
                <tr>
                  <td class="content-cell">
                    <h1>{{if .Email.Body.Title }}{{ .Email.Body.Title }}{{ else if .Email.Body.Name }}{{ .Email.Body.Greeting }} {{ .Email.Body.Name }},{{ else }}{{ .Email.Body.Greeting }},{{ end }}</h1>
                    {{ with .Email.Body.Intros }}
                        {{ if gt (len .) 0 }}
                          {{ range $line := . }}
                            <p>{{ $line }}</p>
                          {{ end }}
                        {{ end }}
                    {{ end }}
                    {{ if (ne .Email.Body.FreeMarkdown "") }}
                      {{ .Email.Body.FreeMarkdown.ToHTML }}
                    {{ else }}
                      {{ with .Email.Body.Dictionary }}
                        {{ if gt (len .) 0 }}
                          <dl class="body-dictionary">
                            {{ range $entry := . }}
                              <dt>{{ $entry.Key }}:</dt>
                              <dd>{{ $entry.Value }}</dd>
                            {{ end }}
                          </dl>
                        {{ end }}
                      {{ end }}
                      <!-- Table -->
                      {{ with .Email.Body.Table }}
                        {{ $data := .Data }}
                        {{ $columns := .Columns }}
                        {{ if gt (len $data) 0 }}
                          <table class="data-wrapper" width="100%" cellpadding="0" cellspacing="0">
                            <tr>
                              <td colspan="2">
                                <table class="data-table" width="100%" cellpadding="0" cellspacing="0">
                                  <tr>
                                    {{ $col := index $data 0 }}
                                    {{ range $entry := $col }}
                                      <th
                                        {{ with $columns }}
                                          {{ $width := index .CustomWidth $entry.Key }}
                                          {{ with $width }}
                                            width="{{ . }}"
                                          {{ end }}
                                          {{ $align := index .CustomAlignement $entry.Key }}
                                          {{ with $align }}
                                            style="text-align:{{ . }}"
                                          {{ end }}
                                        {{ end }}
                                      >
                                        <p>{{ $entry.Key }}</p>
                                      </th>
                                    {{ end }}
                                  </tr>
                                  {{ range $row := $data }}
                                    <tr>
                                      {{ range $cell := $row }}
                                        <td
                                          {{ with $columns }}
                                            {{ $align := index .CustomAlignement $cell.Key }}
                                            {{ with $align }}
                                              style="text-align:{{ . }}"
                                            {{ end }}
                                          {{ end }}
                                        >
                                          {{ $cell.Value }}
                                        </td>
                                      {{ end }}
                                    </tr>
                                  {{ end }}
                                </table>
                              </td>
                            </tr>
                          </table>
                        {{ end }}
                      {{ end }}
                      <!-- Action -->
                      {{ with .Email.Body.Actions }}
                        {{ if gt (len .) 0 }}
                          {{ range $action := . }}
                            {{$instr := splitList "\n" $action.Instructions}}
                            {{range $i, $p := $instr}}
                              {{if ne $p ""}}
                              <p>{{$p}}</p>
                              {{end}}
                            {{end}}
                            <table class="body-action" align="center" width="100%" cellpadding="0" cellspacing="0">
                              <tr>
                                <td align="center">
                                  <div>
                                    <a href="{{ $action.Button.Link }}" class="button" style="background-color: {{ $action.Button.Color }}" target="_blank">
                                      {{ $action.Button.Text }}
                                    </a>
                                  </div>
                                </td>
                              </tr>
                            </table>
                          {{ end }}
                        {{ end }}
                      {{ end }}
                    {{ end }}
                    {{ with .Email.Body.Outros }}
                        {{ if gt (len .) 0 }}
                          {{ range $line := . }}
                            <p>{{ $line }}</p>
                          {{ end }}
                        {{ end }}
                      {{ end }}
                    <p>
                      {{.Email.Body.Signature}},
                      <br />
                      {{.Hermes.Product.Name}}
                    </p>
                    {{ if (eq .Email.Body.FreeMarkdown "") }}
                      {{ with .Email.Body.Actions }}
                        <table class="body-sub">
                          <tbody>
                              {{ range $action := . }}
                                <tr>
                                  <td>
                                    <p class="sub">{{$.Hermes.Product.TroubleText | replace "{ACTION}" $action.Button.Text}}</p>
                                    <p class="sub"><a href="{{ $action.Button.Link }}">{{ $action.Button.Link }}</a></p>
                                  </td>
                                </tr>
                              {{ end }}
                          </tbody>
                        </table>
                      {{ end }}
                    {{ end }}
                  </td>
                </tr>
              </table>
            </td>
          </tr>
          <tr>
            <td>
              <table class="email-footer" align="center" width="570" cellpadding="0" cellspacing="0">
                <tr>
                  <td class="content-cell">
                    <p class="sub center">
                      <!-- {{.Hermes.Product.Copyright}} -->
                    </p>
                  </td>
                </tr>
              </table>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>
`
}

// PlainTextTemplate returns a Golang template that will generate an plain text email.
func (dt *MailTheme) PlainTextTemplate() string {
	return `<h2>{{if .Email.Body.Title }}{{ .Email.Body.Title }}{{ else if .Email.Body.Name }}{{ .Email.Body.Greeting }} {{ .Email.Body.Name }},{{else}}{{ .Email.Body.Greeting }},{{ end }}</h2>
{{ with .Email.Body.Intros }}
  {{ range $line := . }}
    <p>{{ $line }}</p>
  {{ end }}
{{ end }}
{{ if (ne .Email.Body.FreeMarkdown "") }}
  {{ .Email.Body.FreeMarkdown.ToHTML }}
{{ else }}
  {{ with .Email.Body.Dictionary }}
    <ul>
    {{ range $entry := . }}
      <li>{{ $entry.Key }}: {{ $entry.Value }}</li>
    {{ end }}
    </ul>
  {{ end }}
  {{ with .Email.Body.Table }}
    {{ $data := .Data }}
    {{ $columns := .Columns }}
    {{ if gt (len $data) 0 }}
      <table class="data-table" width="100%" cellpadding="0" cellspacing="0">
        <tr>
          {{ $col := index $data 0 }}
          {{ range $entry := $col }}
            <th>{{ $entry.Key }} </th>
          {{ end }}
        </tr>
        {{ range $row := $data }}
          <tr>
            {{ range $cell := $row }}
              <td>
                {{ $cell.Value }}
              </td>
            {{ end }}
          </tr>
        {{ end }}
      </table>
    {{ end }}
  {{ end }}
  {{ with .Email.Body.Actions }}
    {{ range $action := . }}
      <p>{{ $action.Instructions }} {{ $action.Button.Link }}</p>
    {{ end }}
  {{ end }}
{{ end }}
{{ with .Email.Body.Outros }}
  {{ range $line := . }}
    <p>{{ $line }}<p>
  {{ end }}
{{ end }}
<p>{{.Email.Body.Signature}},<br>{{.Hermes.Product.Name}} - {{.Hermes.Product.Link}}</p>
`
}
