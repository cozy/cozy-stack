package push

import (
	"crypto/ecdsa"
	"crypto/md5"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/oauth"

	multierror "github.com/hashicorp/go-multierror"

	fcm "github.com/appleboy/go-fcm"

	apns "github.com/sideshow/apns2"
	apns_cert "github.com/sideshow/apns2/certificate"
	apns_payload "github.com/sideshow/apns2/payload"
	apns_token "github.com/sideshow/apns2/token"
)

var (
	fcmClient *fcm.Client
	iosClient *apns.Client
)

func init() {
	jobs.AddWorker(&jobs.WorkerConfig{
		WorkerType:   "push",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 2,
		Timeout:      10 * time.Second,
		WorkerInit:   Init,
		WorkerFunc:   Worker,
	})
}

// Message contains a push notification request.
type Message struct {
	NotificationID string `json:"notification_id"`
	Source         string `json:"source"`
	ClientID       string `json:"client_id,omitempty"`
	Platform       string `json:"platform,omitempty"`
	DeviceToken    string `json:"device_token,omitempty"`
	Title          string `json:"title,omitempty"`
	Message        string `json:"message,omitempty"`
	Priority       string `json:"priority,omitempty"`
	Sound          string `json:"sound,omitempty"`
	Collapsible    bool   `json:"collapsible,omitempty"`

	Data map[string]interface{} `json:"data,omitempty"`
}

// Init initializes the necessary global clients
func Init() (err error) {
	conf := config.GetConfig().Notifications

	if conf.AndroidAPIKey != "" {
		fcmClient, err = fcm.NewClient(conf.AndroidAPIKey)
		if err != nil {
			return
		}
	}

	if conf.IOSCertificateKeyPath != "" {
		var authKey *ecdsa.PrivateKey
		var certificateKey tls.Certificate

		switch filepath.Ext(conf.IOSCertificateKeyPath) {
		case ".p12":
			certificateKey, err = apns_cert.FromP12File(
				conf.IOSCertificateKeyPath, conf.IOSCertificatePassword)
		case ".pem":
			certificateKey, err = apns_cert.FromPemFile(
				conf.IOSCertificateKeyPath, conf.IOSCertificatePassword)
		case ".p8":
			authKey, err = apns_token.AuthKeyFromFile(conf.IOSCertificateKeyPath)
		default:
			err = errors.New("wrong certificate key extension")
		}
		if err != nil {
			return err
		}

		if authKey != nil {
			t := &apns_token.Token{
				AuthKey: authKey,
				KeyID:   conf.IOSKeyID,
				TeamID:  conf.IOSTeamID,
			}
			iosClient = apns.NewTokenClient(t)
		} else {
			iosClient = apns.NewClient(certificateKey)
		}
		if conf.Development {
			iosClient = iosClient.Development()
		} else {
			iosClient = iosClient.Production()
		}
	}
	return
}

// Worker is the worker that just logs its message (useful for debugging)
func Worker(ctx *jobs.WorkerContext) error {
	var msg Message
	if err := ctx.UnmarshalMessage(&msg); err != nil {
		return err
	}
	if msg.ClientID != "" {
		inst, err := instance.Get(ctx.Domain())
		if err != nil {
			return err
		}
		c, err := oauth.FindClient(inst, msg.ClientID)
		if err != nil {
			return err
		}
		msg.Platform = c.NotificationPlatform
		msg.DeviceToken = c.NotificationDeviceToken
	}
	switch msg.Platform {
	case oauth.AndroidPlatform:
		return pushToAndroid(ctx, &msg)
	case oauth.IOSPlatform:
		return pushToIOS(ctx, &msg)
	default:
		return fmt.Errorf("notifications: unknown platform %q", msg.Platform)
	}
}

// Firebase Cloud Messaging HTTP Protocol
// https://firebase.google.com/docs/cloud-messaging/http-server-ref
func pushToAndroid(ctx *jobs.WorkerContext, msg *Message) error {
	if fcmClient == nil {
		ctx.Logger().Warn("Could not send android notification: not configured")
		return nil
	}

	var priority string
	if msg.Priority == "high" {
		priority = "high"
	}

	var hashedSource []byte
	if msg.Collapsible {
		hashedSource = hashSource(msg.Source)
	} else {
		hashedSource = hashSource(msg.Source + msg.NotificationID)
	}

	// notID should be an integer, we take the first 32bits of the hashed source
	// value.
	notID := int32(binary.BigEndian.Uint32(hashedSource[:4]))
	if notID < 0 {
		notID = -notID
	}

	notification := &fcm.Message{
		To:               msg.DeviceToken,
		Priority:         priority,
		ContentAvailable: true,
		Notification:     &fcm.Notification{Sound: msg.Sound},
		Data: map[string]interface{}{
			// Fields required by phonegap-plugin-push
			// see: https://github.com/phonegap/phonegap-plugin-push/blob/master/docs/PAYLOAD.md#android-behaviour
			"notId": notID,

			"title": msg.Title,
			"body":  msg.Message,
		},
	}
	if msg.Collapsible {
		notification.CollapseKey = hex.EncodeToString(hashedSource)
	}
	for k, v := range msg.Data {
		notification.Data[k] = v
	}

	res, err := fcmClient.Send(notification)
	if err != nil {
		return err
	}
	if res.Failure == 0 {
		return nil
	}

	var errm error
	for _, result := range res.Results {
		if err = result.Error; err != nil {
			if errm != nil {
				errm = multierror.Append(errm, err)
			} else {
				errm = err
			}
		}
	}

	return errm
}

func pushToIOS(ctx *jobs.WorkerContext, msg *Message) error {
	if iosClient == nil {
		ctx.Logger().Warn("Could not send iOS notification: not configured")
		return nil
	}

	var priority int
	if msg.Priority == "normal" {
		priority = apns.PriorityLow
	} else {
		priority = apns.PriorityHigh
	}

	payload := apns_payload.NewPayload().
		AlertTitle(msg.Title).
		Alert(msg.Message).
		Sound(msg.Sound)

	for k, v := range msg.Data {
		payload.Custom(k, v)
	}

	notification := &apns.Notification{
		DeviceToken: msg.DeviceToken,
		Payload:     payload,
		Priority:    priority,
		CollapseID:  hex.EncodeToString(hashSource(msg.Source)), // CollapseID should not exceed 64 bytes
	}

	res, err := iosClient.PushWithContext(ctx, notification)
	if err != nil {
		return err
	}
	if res.StatusCode != 200 {
		return fmt.Errorf("failed to push apns notification: %d %s", res.StatusCode, res.Reason)
	}
	return nil
}

func hashSource(source string) []byte {
	h := md5.New()
	h.Write([]byte(source))
	return h.Sum(nil)
}
