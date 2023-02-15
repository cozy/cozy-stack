package office

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	jwt "github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
)

// SendSaveMessage is used by the trigger for asking OO to save the document in
// the Cozy.
type SendSaveMessage struct {
	Key string `json:"key"`
}

func setupTrigger(inst *instance.Instance, key string) error {
	sched := job.System()
	infos := job.TriggerInfos{
		Type:       "@every",
		WorkerType: "office-save",
		Arguments:  "10m",
	}
	msg := &SendSaveMessage{Key: key}
	t, err := job.NewTrigger(inst, infos, msg)
	if err != nil {
		return err
	}
	return sched.AddTrigger(t)
}

type commandRequest struct {
	Command  string `json:"c"`
	Key      string `json:"key"`
	Userdata string `json:"userdata"`
	Token    string `json:"token,omitempty"`
}

// Valid is required by the jwt.Claims interface
func (c *commandRequest) Valid() error { return nil }

type commandResponse struct {
	Key   string `json:"key"`
	Error int    `json:"error"`
}

func SendSave(inst *instance.Instance, msg SendSaveMessage) error {
	if _, err := GetStore().GetDoc(inst, msg.Key); err != nil {
		// By returning the ErrBadTrigger code, the stack will know that it
		// must delete the trigger.
		return job.ErrBadTrigger{Err: err}
	}
	cfg := getConfig(inst.ContextName)

	cmd := &commandRequest{
		Command:  "forcesave",
		Key:      msg.Key,
		Userdata: "stack",
	}
	if cfg.InboxSecret != "" {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, cmd)
		signed, err := token.SignedString([]byte(cfg.InboxSecret))
		if err != nil {
			return err
		}
		cmd.Token = signed
	}
	body, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	u, err := url.Parse(cfg.OnlyOfficeURL)
	if err != nil {
		return err
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.Path += "/coauthoring/CommandService.ashx"
	commandURL := u.String()

	res, err := docserverClient.Post(commandURL, echo.MIMEApplicationJSON, bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer func() {
		// Flush the body to allow reusing the connection with Keep-Alive
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
	}()
	var cmdRes commandResponse
	if err := json.NewDecoder(res.Body).Decode(&cmdRes); err != nil {
		return err
	}
	// 0 means OK to save, and 4 means that the doc has not changed
	if cmdRes.Error != 0 && cmdRes.Error != 4 {
		inst.Logger().WithNamespace("office").
			Warnf("error for forcesave %s: %d", msg.Key, cmdRes.Error)
	}
	// 1 and 3 means that something unexpected happens, 1 for OO side, 3 for
	// the stack side: in both cases, we delete the invalid trigger
	if cmdRes.Error == 1 || cmdRes.Error == 3 {
		return job.ErrBadTrigger{Err: errors.New("unexpected state")}
	}
	return nil
}
