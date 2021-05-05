package job

import "sync"

type firer interface {
	fire(trigger Trigger, request *JobRequest)
}

// WebhookTrigger implements the @webhook triggers. It schedules a job when an
// HTTP request is made at this webhook.
type WebhookTrigger struct {
	*TriggerInfos
	mu sync.Mutex
	ch chan *JobRequest
	cb firer
}

// NewWebhookTrigger returns a new instance of WebhookTrigger.
func NewWebhookTrigger(infos *TriggerInfos) (*WebhookTrigger, error) {
	return &WebhookTrigger{TriggerInfos: infos}, nil
}

// Type implements the Type method of the Trigger interface.
func (w *WebhookTrigger) Type() string {
	return w.TriggerInfos.Type
}

// Schedule implements the Schedule method of the Trigger interface.
func (w *WebhookTrigger) Schedule() <-chan *JobRequest {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ch = make(chan *JobRequest)
	return w.ch
}

// Unschedule implements the Unschedule method of the Trigger interface.
func (w *WebhookTrigger) Unschedule() {
	// Note: it is called only for the in-memory scheduler
	w.mu.Lock()
	defer w.mu.Unlock()
	close(w.ch)
	w.ch = nil
}

// Infos implements the Infos method of the Trigger interface.
func (w *WebhookTrigger) Infos() *TriggerInfos {
	return w.TriggerInfos
}

// CombineRequest implements the CombineRequest method of the Trigger interface.
func (w *WebhookTrigger) CombineRequest() string {
	return appendPayload
}

// SetCallback registers a struct to be called when the webhook is fired.
func (w *WebhookTrigger) SetCallback(cb firer) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cb = cb
}

// Fire is called with a payload when the webhook has been requested.
func (w *WebhookTrigger) Fire(payload Payload) {
	w.mu.Lock()
	defer w.mu.Unlock()
	req := w.JobRequest()
	req.Payload = payload
	if w.ch != nil {
		w.ch <- req
	}
	if w.cb != nil {
		w.cb.fire(w, req)
	}
}
