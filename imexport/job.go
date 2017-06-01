package main

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/cozy/cozy-stack/client"
	"github.com/cozy/cozy-stack/client/request"
)

type content struct {
	Type string `json:"type"`
	Body string `json:"body"`
}

type jsonMail struct {
	Mode    string     `json:"mode"`
	Subject string     `json:"subject"`
	Parts   []*content `json:"parts"`
}

type jsonBody struct {
	Data struct {
		Attributes struct {
			Options struct {
				Priority     int `json:"priority"`
				Timeout      int `json:"timeout"`
				MaxExecCount int `json:"max_exec_count"`
			} `json:"options"`
			Arguments *jsonMail `json:"arguments"`
		} `json:"attributes"`
	} `json:"data"`
}

func createMail(body string) ([]byte, error) {

	var tab []*content
	c := &content{
		Type: "text/plain",
		Body: body,
	}
	tab = append(tab, c)
	m := &jsonMail{
		Mode:    "noreply",
		Subject: "Cozy: voici vos documents",
		Parts:   tab,
	}

	b := &jsonBody{}
	b.Data.Attributes.Options.Priority = 3
	b.Data.Attributes.Options.MaxExecCount = 60
	b.Data.Attributes.Options.MaxExecCount = 3
	b.Data.Attributes.Arguments = m

	mail, err := json.Marshal(b)

	return mail, err

}

func sendMail(cClient *client.Client) error {

	text := "Bonjour, vous pouvez des a presents recuperer vos documents"
	mail, err := createMail(text)
	if err != nil {
		return err
	}

	resp, err := cClient.Req(&request.Options{
		Method: "POST",
		Path:   "/jobs/queue/sendmail",
		Body:   bytes.NewReader(mail),
		Headers: request.Headers{
			"Accept": "application/vnd.api+json",
		},
	})

	fmt.Println("mail: ", resp.Status)

	return err
}
