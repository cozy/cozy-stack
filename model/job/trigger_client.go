package job

// ClientTrigger implements the @webhook triggers. It schedules a job when an
// HTTP request is made at this webhook.
type ClientTrigger struct {
	*TriggerInfos
}

// NewClientTrigger returns a new instance of ClientTrigger.
func NewClientTrigger(infos *TriggerInfos) (*ClientTrigger, error) {
	infos.WorkerType = "client" // Force the worker type
	return &ClientTrigger{infos}, nil
}

// Type implements the Type method of the Trigger interface.
func (c *ClientTrigger) Type() string {
	return c.TriggerInfos.Type
}

// Schedule implements the Schedule method of the Trigger interface.
func (c *ClientTrigger) Schedule() <-chan *JobRequest {
	return nil
}

// Unschedule implements the Unschedule method of the Trigger interface.
func (c *ClientTrigger) Unschedule() {}

// Infos implements the Infos method of the Trigger interface.
func (c *ClientTrigger) Infos() *TriggerInfos {
	return c.TriggerInfos
}

// CombineRequest implements the CombineRequest method of the Trigger interface.
func (c *ClientTrigger) CombineRequest() string {
	return keepOriginalRequest
}
