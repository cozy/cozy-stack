package workers

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/jobs"
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

func TestMailSendServer(t *testing.T) {
	clientString := `EHLO localhost
HELO localhost
MAIL FROM:<me@me>
RCPT TO:<you1@you>
DATA
Hey !!!
.
QUIT
`

	expectedHeaders := map[string]string{
		"From":    "me@me",
		"To":      "you1@you",
		"Subject": "Up?",
		"Date":    "Mon, 01 Jan 0001 00:00:00 +0000",
		"Content-Transfer-Encoding": "quoted-printable",
		"Content-Type":              "text/plain; charset=UTF-8",
		"Mime-Version":              "1.0",
	}

	mailServer(t, serverString, clientString, expectedHeaders, func(host string, port int) error {
		msg, err := jobs.NewMessage("json", &MailOptions{
			From: &MailAddress{Mail: "me@me"},
			To: []*MailAddress{
				&MailAddress{Mail: "you1@you"},
			},
			Date:    &time.Time{},
			Subject: "Up?",
			Dialer: &gomail.DialerOptions{
				Host:       host,
				Port:       port,
				DisableTLS: true,
			},
			Parts: []*MailPart{
				&MailPart{
					Body: "Hey !!!",
					Type: "text/plain",
				},
			},
		})
		if err != nil {
			return err
		}
		return SendMail(context.Background(), msg)
	})
}

func TestMailSendTemplateMail(t *testing.T) {
	clientString := `EHLO localhost
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
`

	expectedHeaders := map[string]string{
		"From":    "me@me",
		"To":      "you1@you",
		"Subject": "Up?",
		"Date":    "Mon, 01 Jan 0001 00:00:00 +0000",
		"Content-Transfer-Encoding": "quoted-printable",
		"Content-Type":              "text/html; charset=UTF-8",
		"Mime-Version":              "1.0",
	}

	const tpl = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="UTF-8">
    <title>{{.Title}}</title>
  </head>
  <body>
    {{range .Items}}<div>{{ . }}</div>{{else}}<div><strong>no rows</strong></div>{{end}}
  </body>
</html>`

	data := struct {
		Title string
		Items []string
	}{
		Title: "My page",
		Items: []string{
			"My photos",
			"My blog",
		},
	}

	mailServer(t, serverString, clientString, expectedHeaders, func(host string, port int) error {
		msg, err := jobs.NewMessage("json", &MailOptions{
			From: &MailAddress{Mail: "me@me"},
			To: []*MailAddress{
				&MailAddress{Mail: "you1@you"},
			},
			Date:    &time.Time{},
			Subject: "Up?",
			Dialer: &gomail.DialerOptions{
				Host:       host,
				Port:       port,
				DisableTLS: true,
			},
			Parts: []*MailPart{
				&MailPart{
					Template: tpl,
					Values:   data,
				},
			},
		})
		if err != nil {
			return err
		}
		return SendMail(context.Background(), msg)
	})
}

func TestMailMissingSubject(t *testing.T) {
	msg, err := jobs.NewMessage("json", &MailOptions{
		From: &MailAddress{Mail: "me@me"},
		To:   []*MailAddress{&MailAddress{Mail: "you@you"}},
	})
	if !assert.NoError(t, err) {
		return
	}
	err = SendMail(context.Background(), msg)
	if assert.Error(t, err) {
		assert.Equal(t, "Missing mail subject", err.Error())
	}
}

func TestMailBadBodyType(t *testing.T) {
	msg, err := jobs.NewMessage("json", &MailOptions{
		From:    &MailAddress{Mail: "me@me"},
		To:      []*MailAddress{&MailAddress{Mail: "you@you"}},
		Subject: "Up?",
		Parts: []*MailPart{
			&MailPart{
				Type: "text/qsdqsd",
				Body: "foo",
			},
		},
	})
	if !assert.NoError(t, err) {
		return
	}
	err = SendMail(context.Background(), msg)
	if assert.Error(t, err) {
		assert.Equal(t, "Unknown body content-type text/qsdqsd", err.Error())
	}
}

func TestMailMultiParts(t *testing.T) {
	clientString := `EHLO localhost
HELO localhost
MAIL FROM:<me@me>
RCPT TO:<you1@you>
DATA
Content-Transfer-Encoding: quoted-printable
Content-Type: text/plain; charset=UTF-8
foo
Content-Transfer-Encoding: quoted-printable
Content-Type: text/html; charset=UTF-8
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
`

	expectedHeaders := map[string]string{
		"From":         "me@me",
		"To":           "you1@you",
		"Subject":      "Up?",
		"Date":         "Mon, 01 Jan 0001 00:00:00 +0000",
		"Content-Type": "multipart/alternative;",
		"Mime-Version": "1.0",
	}

	const tpl = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="UTF-8">
    <title>{{.Title}}</title>
  </head>
  <body>
    {{range .Items}}<div>{{ . }}</div>{{else}}<div><strong>no rows</strong></div>{{end}}
  </body>
</html>`

	data := struct {
		Title string
		Items []string
	}{
		Title: "My page",
		Items: []string{
			"My photos",
			"My blog",
		},
	}

	mailServer(t, serverString, clientString, expectedHeaders, func(host string, port int) error {
		msg, err := jobs.NewMessage("json", &MailOptions{
			From: &MailAddress{Mail: "me@me"},
			To: []*MailAddress{
				&MailAddress{Mail: "you1@you"},
			},
			Date:    &time.Time{},
			Subject: "Up?",
			Dialer: &gomail.DialerOptions{
				Host:       host,
				Port:       port,
				DisableTLS: true,
			},
			Parts: []*MailPart{
				&MailPart{
					Type: "text/plain",
					Body: "foo",
				},
				&MailPart{
					Type:     "text/html",
					Template: tpl,
					Values:   data,
				},
			},
		})
		if err != nil {
			return err
		}
		return SendMail(context.Background(), msg)
	})
}

func mailServer(t *testing.T, serverString, clientString string, expectedHeader map[string]string, send func(string, int) error) {
	serverString = strings.Join(strings.Split(serverString, "\n"), "\r\n")
	clientString = strings.Join(strings.Split(clientString, "\n"), "\r\n")

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
	if !assert.Equal(t, clientString, actualcmds) {
		return
	}
	assert.EqualValues(t, expectedHeader, headers)
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	os.Exit(m.Run())
}
