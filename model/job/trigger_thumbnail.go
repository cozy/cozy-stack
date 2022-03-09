package job

import (
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/realtime"
)

type ThumbnailTrigger struct {
	broker      Broker
	log         *logger.Entry
	unscheduled chan struct{}
}

func NewThumbnailTrigger(broker Broker) *ThumbnailTrigger {
	return &ThumbnailTrigger{
		broker:      broker,
		log:         logger.WithNamespace("scheduler"),
		unscheduled: make(chan struct{}),
	}
}

func (t *ThumbnailTrigger) Schedule() {
	sub := realtime.GetHub().SubscribeLocalAll()
	defer sub.Close()
	for {
		select {
		case e := <-sub.Channel:
			if t.match(e) {
				t.pushJob(e)
			}
		case <-t.unscheduled:
			return
		}
	}
}

func (t *ThumbnailTrigger) match(e *realtime.Event) bool {
	if e.Doc.DocType() != consts.Files {
		return false
	}
	if e.Verb == realtime.EventNotify {
		return false
	}

	if doc, ok := e.Doc.(permission.Fetcher); ok {
		for _, class := range doc.Fetch("class") {
			if class == "image" || class == "pdf" {
				return true
			}
		}
	}
	return false
}

func (t *ThumbnailTrigger) pushJob(e *realtime.Event) {
	event, err := NewEvent(e)
	if err != nil {
		return
	}
	req := &JobRequest{
		WorkerType: "thumbnail",
		Message:    Message("{}"),
		Event:      event,
	}
	log := t.log.WithField("domain", e.Domain)
	log.Infof("trigger thumbnail: Pushing new job")
	if _, err := t.broker.PushJob(e, req); err != nil {
		log.Errorf("trigger thumbnail: Could not schedule a new job: %s", err.Error())
	}
}

func (t *ThumbnailTrigger) Unschedule() {
	close(t.unscheduled)
}
