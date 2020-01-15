package center

import "github.com/cozy/cozy-stack/pkg/mail"

// PushMessage contains a push notification request.
type PushMessage struct {
	NotificationID string `json:"notification_id"`
	Source         string `json:"source"`
	Title          string `json:"title,omitempty"`
	Message        string `json:"message,omitempty"`
	Priority       string `json:"priority,omitempty"`
	Sound          string `json:"sound,omitempty"`
	Collapsible    bool   `json:"collapsible,omitempty"`

	Data map[string]interface{} `json:"data,omitempty"`

	MailFallback *mail.Options `json:"mail_fallback,omitempty"`
}
