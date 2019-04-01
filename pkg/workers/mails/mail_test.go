package mails

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"net"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/gomail"
	"github.com/stretchr/testify/assert"
)

const serverString = `220 hello world
502 EH?
250 smtp.me at your service
250 Sender ok
250 Receiver ok
354 Go ahead
250 Data ok
221 Goodbye
`

var inst *instance.Instance

func TestMailSendServer(t *testing.T) {
	clientStrings := []string{`EHLO localhost
HELO localhost
MAIL FROM:<me@me>
RCPT TO:<you1@you>
DATA
Hey !!!
.
QUIT
`}

	expectedHeaders := map[string]string{
		"From":                      "me@me",
		"To":                        "you1@you",
		"Subject":                   "Up?",
		"Date":                      "Mon, 01 Jan 0001 00:00:00 +0000",
		"Content-Transfer-Encoding": "quoted-printable",
		"Content-Type":              "text/plain; charset=UTF-8",
		"Mime-Version":              "1.0",
		"X-Cozy":                    "cozy.example.com",
	}

	mailServer(t, serverString, clientStrings, expectedHeaders, func(host string, port int) error {
		msg := &Options{
			From: &Address{Email: "me@me"},
			To: []*Address{
				{Email: "you1@you"},
			},
			Date:    &time.Time{},
			Subject: "Up?",
			Dialer: &gomail.DialerOptions{
				Host:       host,
				Port:       port,
				DisableTLS: true,
			},
			Parts: []*Part{
				{
					Body: "Hey !!!",
					Type: "text/plain",
				},
			},
			Locale: "en",
		}
		return sendMail(context.Background(), msg, "cozy.example.com")
	})
}

func TestMailSendTemplateMail(t *testing.T) {
	clientStrings := []string{`EHLO localhost
HELO localhost
MAIL FROM:<me@me>
RCPT TO:<you1@you>
DATA
<!DOCTYPE html>
<html>
  <head>
    <meta charset=3D"UTF-8">
    <title>My page</title>
  </head>
  <body>
    <div>My photos</div><div>My blog</div>
  </body>
</html>
.
QUIT
`}

	expectedHeaders := map[string]string{
		"From":                      "me@me",
		"To":                        "you1@you",
		"Subject":                   "Up?",
		"Date":                      "Mon, 01 Jan 0001 00:00:00 +0000",
		"Content-Transfer-Encoding": "quoted-printable",
		"Content-Type":              "text/html; charset=UTF-8",
		"Mime-Version":              "1.0",
		"X-Cozy":                    "cozy.example.com",
	}

	mailBody := `<!DOCTYPE html>
<html>
  <head>
    <meta charset="UTF-8">
    <title>My page</title>
  </head>
  <body>
    <div>My photos</div><div>My blog</div>
  </body>
</html>
`

	mailServer(t, serverString, clientStrings, expectedHeaders, func(host string, port int) error {
		msg := &Options{
			From: &Address{Email: "me@me"},
			To: []*Address{
				{Email: "you1@you"},
			},
			Date:    &time.Time{},
			Subject: "Up?",
			Dialer: &gomail.DialerOptions{
				Host:       host,
				Port:       port,
				DisableTLS: true,
			},
			Parts: []*Part{
				{Body: mailBody, Type: "text/html"},
			},
			Locale: "en",
		}
		return sendMail(context.Background(), msg, "cozy.example.com")
	})
}

func TestMailMissingSubject(t *testing.T) {
	msg := &Options{
		From:   &Address{Email: "me@me"},
		To:     []*Address{{Email: "you@you"}},
		Locale: "en",
	}
	err := sendMail(context.Background(), msg, "cozy.example.com")
	if assert.Error(t, err) {
		assert.Equal(t, "Missing mail subject", err.Error())
	}
}

func TestMailBadBodyType(t *testing.T) {
	msg := &Options{
		From:    &Address{Email: "me@me"},
		To:      []*Address{{Email: "you@you"}},
		Subject: "Up?",
		Parts: []*Part{
			{
				Type: "text/qsdqsd",
				Body: "foo",
			},
		},
		Locale: "en",
	}
	err := sendMail(context.Background(), msg, "cozy.example.com")
	if assert.Error(t, err) {
		assert.Equal(t, "Unknown body content-type text/qsdqsd", err.Error())
	}
}

func TestMailMultiParts(t *testing.T) {
	clientStrings := []string{`EHLO localhost
HELO localhost
MAIL FROM:<me@me>
RCPT TO:<you1@you>
DATA
Content-Transfer-Encoding: quoted-printable
Content-Type: text/html; charset=UTF-8`,
		`Content-Transfer-Encoding: quoted-printable
Content-Type: text/plain; charset=UTF-8
Mail Greeting,
intro1
intro2 My page
instructions https://foobar.baz
outro1
outro2 My page
Mail Signature,
Mail Cozy Team - https://cozy.io
.
QUIT
`}

	expectedHeaders := map[string]string{
		"From":         "me@me",
		"To":           "you1@you",
		"Subject":      "Up?",
		"Date":         "Mon, 01 Jan 0001 00:00:00 +0000",
		"Content-Type": "multipart/alternative;",
		"Mime-Version": "1.0",
		"X-Cozy":       "cozy.example.com",
	}

	oldMailTemplater := mailTemplater
	mailTemplater = &MailTemplater{[]*MailTemplate{
		{
			Name:    "test",
			Subject: "Up?",
			Intro:   "intro1\nintro2 {{.Title}}",
			Outro:   "outro1\noutro2 {{.Title}}",
			Actions: []MailAction{
				{
					Instructions: "instructions",
					Text:         "button",
					Link:         "{{.Link}}",
				},
			},
		},
	}}
	defer func() {
		mailTemplater = oldMailTemplater
	}()

	data := struct {
		Title string
		Link  string
	}{
		Title: "My page",
		Link:  "https://foobar.baz",
	}

	mailServer(t, serverString, clientStrings, expectedHeaders, func(host string, port int) error {
		msg := &Options{
			From: &Address{Email: "me@me"},
			To: []*Address{
				{Email: "you1@you"},
			},
			Date:    &time.Time{},
			Subject: "Up?",
			Dialer: &gomail.DialerOptions{
				Host:       host,
				Port:       port,
				DisableTLS: true,
			},
			TemplateName:   "test",
			TemplateValues: data,
			Locale:         "en",
		}
		return sendMail(context.Background(), msg, "cozy.example.com")
	})
}

func mailServer(t *testing.T, serverString string, clientStrings []string, expectedHeader map[string]string, send func(string, int) error) {
	serverString = strings.Join(strings.Split(serverString, "\n"), "\r\n")
	for i, s := range clientStrings {
		clientStrings[i] = strings.Join(strings.Split(s, "\n"), "\r\n")
	}

	var cmdbuf bytes.Buffer
	bcmdbuf := bufio.NewWriter(&cmdbuf)
	headers := make(map[string]string)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Unable to to create listener: %v", err)
	}
	defer l.Close()

	// prevent data race on bcmdbuf
	var done = make(chan struct{})
	go func(data []string) {

		defer close(done)

		conn, err := l.Accept()
		if err != nil {
			t.Errorf("Accept error: %v", err)
			return
		}
		defer conn.Close()

		tc := textproto.NewConn(conn)
		readdata := false
		readhead := false
		for i := 0; i < len(data) && data[i] != ""; i++ {
			tc.PrintfLine(data[i])
			for len(data[i]) >= 4 && data[i][3] == '-' {
				i++
				tc.PrintfLine(data[i])
			}
			if data[i] == "221 Goodbye" {
				return
			}
			read := false
			for !read || data[i] == "354 Go ahead" {
				msg, err := tc.ReadLine()
				if readdata && msg != "." {
					if msg == "" {
						readhead = true
						read = true
						continue
					}
					// skip multipart --boundaries
					if readhead &&
						(len(msg) <= 1 || msg[0] != '-' || msg[1] != '-') {
						bcmdbuf.Write([]byte(msg + "\r\n"))
					} else {
						parts := strings.SplitN(msg, ": ", 2)
						if len(parts) == 2 {
							headers[parts[0]] = parts[1]
						}
					}
				} else {
					if msg == "." {
						readdata = false
					}
					if msg == "DATA" {
						readdata = true
					}
					bcmdbuf.Write([]byte(msg + "\r\n"))
					read = true
				}
				if err != nil {
					t.Errorf("Read error: %v", err)
					return
				}
				if data[i] == "354 Go ahead" && msg == "." {
					break
				}
			}
		}
	}(strings.Split(serverString, "\r\n"))

	host, port, _ := net.SplitHostPort(l.Addr().String())
	portI, _ := strconv.Atoi(port)
	if err := send(host, portI); err != nil {
		t.Errorf("%v", err)
	}

	<-done
	bcmdbuf.Flush()
	actualcmds := cmdbuf.String()
	for _, s := range clientStrings {
		assert.Contains(t, actualcmds, s)
	}
	assert.EqualValues(t, expectedHeader, headers)
}

func TestSendMailNoReply(t *testing.T) {
	sendMail = func(ctx context.Context, opts *Options, domain string) error {
		assert.NotNil(t, opts.From)
		assert.NotNil(t, opts.To)
		assert.Len(t, opts.To, 1)
		assert.Equal(t, "me@me", opts.To[0].Email)
		assert.Equal(t, "noreply@"+inst.Domain, opts.From.Email)
		assert.Equal(t, inst.Domain, domain)
		return errors.New("yes")
	}
	defer func() {
		sendMail = doSendMail
	}()
	msg, _ := jobs.NewMessage(Options{
		Mode:    "noreply",
		Subject: "Up?",
		Parts: []*Part{
			{
				Type: "text/plain",
				Body: "foo",
			},
		},
		Locale: "en",
	})
	j := jobs.NewJob(inst, &jobs.JobRequest{
		Message:    msg,
		WorkerType: "sendmail",
	})
	err := SendMail(jobs.NewWorkerContext("123", j, inst))
	if assert.Error(t, err) {
		assert.Equal(t, "yes", err.Error())
	}
}

func TestSendMailFrom(t *testing.T) {
	sendMail = func(ctx context.Context, opts *Options, domain string) error {
		assert.NotNil(t, opts.From)
		assert.NotNil(t, opts.To)
		assert.Len(t, opts.To, 1)
		assert.Equal(t, "you@you", opts.To[0].Email)
		assert.Equal(t, "noreply@"+inst.Domain, opts.From.Email)
		assert.Equal(t, "me@me", opts.ReplyTo.Email)
		assert.Equal(t, inst.Domain, domain)
		return errors.New("yes")
	}
	defer func() {
		sendMail = doSendMail
	}()
	msg, _ := jobs.NewMessage(Options{
		Mode:    "from",
		Subject: "Up?",
		To:      []*Address{{Email: "you@you"}},
		Parts: []*Part{
			{
				Type: "text/plain",
				Body: "foo",
			},
		},
		Locale: "en",
	})
	j := jobs.NewJob(inst, &jobs.JobRequest{
		Message:    msg,
		WorkerType: "sendmail",
	})
	err := SendMail(jobs.NewWorkerContext("123", j, inst))
	if assert.Error(t, err) {
		assert.Equal(t, "yes", err.Error())
	}
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	setup := testutils.NewSetup(m, "mails_test")
	inst = setup.GetTestInstance(&lifecycle.Options{Email: "me@me"})
	os.Exit(m.Run())
}
