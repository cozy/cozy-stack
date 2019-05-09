package center

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
}
