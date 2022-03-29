// Package huawei can be used to send notifications via the Huawei Push Kit APIs.
// https://developer.huawei.com/consumer/en/doc/development/HMSCore-References/https-send-api-0000001050986197
package huawei

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/labstack/echo/v4"
)

// Client can be used to send notifications via the Huawei Push Kit APIs.
type Client struct {
	getTokenURL     string
	sendMessagesURL string

	// Access token fields
	token struct {
		mu     sync.Mutex
		expire time.Time
		value  string
	}
}

// NewClient create a client for sending notifications.
func NewClient(conf config.Notifications) (*Client, error) {
	_, err := url.Parse(conf.HuaweiSendMessagesURL)
	if err != nil {
		return nil, fmt.Errorf("cannot parse huawei_send_message: %s", err)
	}
	_, err = url.Parse(conf.HuaweiGetTokenURL)
	if err != nil {
		return nil, fmt.Errorf("cannot parse huawei_get_token: %s", err)
	}
	client := Client{
		getTokenURL:     conf.HuaweiGetTokenURL,
		sendMessagesURL: conf.HuaweiSendMessagesURL,
	}
	return &client, nil
}

// Notification is the payload to send to Push Kit for sending a notification.
// Cf https://developer.huawei.com/consumer/en/doc/development/HMSCore-References/https-send-api-0000001050986197#section13271045101216
type Notification struct {
	Message NotificationMessage `json:"message"`
}

type NotificationMessage struct {
	Android AndroidStructure `json:"android"`
	Token   []string         `json:"token"`
}

type AndroidStructure struct {
	Data         string                `json:"data"`
	Notification NotificationStructure `json:"notification"`
}

type NotificationStructure struct {
	Title       string         `json:"title"`
	Body        string         `json:"body"`
	ClickAction ClickStructure `json:"click_action"`
}

type ClickStructure struct {
	Type int `json:"type"`
}

func NewNotification(title, body, token string, data map[string]interface{}) *Notification {
	notif := &Notification{
		Message: NotificationMessage{
			Android: AndroidStructure{
				Notification: NotificationStructure{
					Title:       title,
					Body:        body,
					ClickAction: ClickStructure{Type: 3},
				},
			},
			Token: []string{token},
		},
	}
	if serializedData, err := json.Marshal(data); err == nil {
		notif.Message.Android.Data = string(serializedData)
	}
	return notif
}

// PushWithContext send the notification to Push Kit.
func (c *Client) PushWithContext(ctx context.Context, notification *Notification) error {
	token, err := c.fetchAccessToken()
	if err != nil {
		return err
	}

	payload, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("cannot marshal notification: %s", err)
	}
	body := bytes.NewBuffer(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.sendMessagesURL, body)
	if err != nil {
		return fmt.Errorf("cannot make request: %s", err)
	}
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("cannot send notification: %s", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		var data map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&data); err == nil {
			logger.WithNamespace("huawei").
				Infof("Failed to send notification (%d): %#v", res.StatusCode, data)
		}
		return fmt.Errorf("cannot send notification: bad code %d", res.StatusCode)
	}
	return nil
}

type accessTokenResponse struct {
	Value string `json:"accessToken"`
}

func (c *Client) fetchAccessToken() (string, error) {
	c.token.mu.Lock()
	defer c.token.mu.Unlock()

	now := time.Now()
	if c.token.expire.After(now) {
		return c.token.value, nil
	}

	res, err := http.Get(c.getTokenURL)
	if err != nil {
		return "", fmt.Errorf("cannot fetch access token: %s", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("cannot fetch access token: bad code %d", res.StatusCode)
	}

	var token accessTokenResponse
	if err := json.NewDecoder(res.Body).Decode(&token); err != nil {
		return "", fmt.Errorf("cannot parse access token response: %s", err)
	}

	c.token.expire = now.Add(55 * time.Minute)
	c.token.value = token.Value
	return token.Value, nil
}
