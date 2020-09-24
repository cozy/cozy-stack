package center

import "github.com/cozy/cozy-stack/pkg/mail"

// SMS contains a notification request for sending a SMS.
type SMS struct {
	NotificationID string        `json:"notification_id"`
	Message        string        `json:"message,omitempty"`
	MailFallback   *mail.Options `json:"mail_fallback,omitempty"`
}
