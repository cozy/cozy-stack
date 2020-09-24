package sms

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/notification/center"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/mail"
	"github.com/sirupsen/logrus"
)

func init() {
	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "sms",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 1,
		Timeout:      10 * time.Second,
		Reserved:     true,
		WorkerFunc:   Worker,
	})
}

// Worker is the worker that send SMS.
func Worker(ctx *job.WorkerContext) error {
	var msg center.SMS
	if err := ctx.UnmarshalMessage(&msg); err != nil {
		return err
	}

	err := sendSMS(ctx, &msg)
	if err != nil {
		ctx.Logger().Warnf("could not send SMS notification: %s", err)
		sendFallbackMail(ctx.Instance, msg.MailFallback)
	}
	return err
}

func sendSMS(ctx *job.WorkerContext, msg *center.SMS) error {
	inst := ctx.Instance
	cfg, err := getConfig(inst)
	if err != nil {
		return err
	}
	number, err := getMyselfPhoneNumber(inst)
	if err != nil {
		return err
	}
	switch cfg.Provider {
	case "api_sen":
		log := ctx.Logger()
		return sendSenAPI(cfg, msg, number, log)
	}
	return errors.New("Unknown provider for sending SMS")
}

func sendSenAPI(cfg *config.SMS, msg *center.SMS, number string, log *logrus.Entry) error {
	payload, err := json.Marshal(map[string]interface{}{
		"content":  msg.Message,
		"receiver": []interface{}{number},
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, cfg.URL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+cfg.Token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == 200 {
		return nil
	}

	var body map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&body); err == nil {
		if t, ok := body["type"].(string); ok {
			log = log.WithField("type", t)
		}
		if detail, ok := body["detail"].(string); ok {
			log = log.WithField("detail", detail)
		}
		log.WithField("status_code", res.StatusCode).Warnf("Cannot send SMS")
	}
	return fmt.Errorf("Unexpected status code: %d", res.StatusCode)
}

func getMyselfPhoneNumber(inst *instance.Instance) (string, error) {
	myself, err := contact.GetMyself(inst)
	if err != nil {
		return "", err
	}
	number := myself.PrimaryPhoneNumber()
	if number == "" {
		return "", errors.New("No phone number in the myself contact document")
	}
	return number, nil
}

func getConfig(inst *instance.Instance) (*config.SMS, error) {
	cfg, ok := config.GetConfig().Notifications.Contexts[inst.ContextName]
	if !ok {
		return nil, errors.New("SMS not configured on this context")
	}
	return &cfg, nil
}

func sendFallbackMail(inst *instance.Instance, email *mail.Options) {
	if inst == nil || email == nil {
		return
	}
	msg, err := job.NewMessage(&email)
	if err != nil {
		return
	}
	_, _ = job.System().PushJob(inst, &job.JobRequest{
		WorkerType: "sendmail",
		Message:    msg,
	})
}
