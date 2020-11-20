package job

// WebhookTrigger implements the @webhook triggers. It schedules a job when an
// HTTP request is made at this webhook.
type WebhookTrigger struct {
	*TriggerInfos
}

// NewWebhookTrigger returns a new instance of WebhookTrigger.
func NewWebhookTrigger(infos *TriggerInfos) (*WebhookTrigger, error) {
	return &WebhookTrigger{infos}, nil
}

// Type implements the Type method of the Trigger interface.
func (w *WebhookTrigger) Type() string {
	return w.TriggerInfos.Type
}

// Schedule implements the Schedule method of the Trigger interface.
func (w *WebhookTrigger) Schedule() <-chan *JobRequest {
	return nil
}

// Unschedule implements the Unschedule method of the Trigger interface.
func (w *WebhookTrigger) Unschedule() {}

// Infos implements the Infos method of the Trigger interface.
func (w *WebhookTrigger) Infos() *TriggerInfos {
	return w.TriggerInfos
}
