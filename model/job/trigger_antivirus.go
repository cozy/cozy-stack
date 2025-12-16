package job

import (
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/realtime"
)

// AntivirusTrigger listens for file creation events and schedules antivirus scans.
type AntivirusTrigger struct {
	broker      Broker
	log         *logger.Entry
	unscheduled chan struct{}
}

// NewAntivirusTrigger creates a new antivirus trigger.
func NewAntivirusTrigger(broker Broker) *AntivirusTrigger {
	return &AntivirusTrigger{
		broker:      broker,
		log:         logger.WithNamespace("scheduler"),
		unscheduled: make(chan struct{}),
	}
}

// Schedule starts listening for file creation events.
func (t *AntivirusTrigger) Schedule() {
	sub := realtime.GetHub().SubscribeFirehose()
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

func (t *AntivirusTrigger) match(e *realtime.Event) bool {
	// Only match file creation events
	if e.Doc.DocType() != consts.Files {
		return false
	}
	if e.Verb != realtime.EventCreate {
		return false
	}
	// Check if it's a file (not directory)
	if doc, ok := e.Doc.(*vfs.FileDoc); ok {
		return doc.Type == consts.FileType
	}
	t.log.Warnf("Event doc is not *vfs.FileDoc, actual type: %T", e.Doc)
	return false
}

func (t *AntivirusTrigger) pushJob(e *realtime.Event) {
	// Get instance to check if antivirus is enabled
	inst, err := instance.Get(e.Domain)
	if err != nil {
		return
	}

	cfg := config.GetAntivirusConfig(inst.ContextName)
	if cfg == nil || !cfg.Enabled {
		return
	}

	doc, ok := e.Doc.(*vfs.FileDoc)
	if !ok {
		return
	}

	log := t.log.WithField("domain", e.Domain)

	t.setPendingStatus(inst, doc, log)

	// Push antivirus job
	event, err := NewEvent(e)
	if err != nil {
		log.Errorf("trigger antivirus: Could not create event for file %s: %s", doc.DocID, err.Error())
		return
	}

	msg, _ := NewMessage(map[string]string{"file_id": doc.DocID})
	req := &JobRequest{
		WorkerType: "antivirus",
		Message:    msg,
		Event:      event,
	}

	log.Infof("trigger antivirus: Pushing scan job for file %s", doc.DocID)
	if _, err := t.broker.PushJob(inst, req); err != nil {
		log.Errorf("trigger antivirus: Could not schedule job: %s", err.Error())
	}
}

func (t *AntivirusTrigger) setPendingStatus(inst *instance.Instance, doc *vfs.FileDoc, log logger.Logger) {
	fs := inst.VFS()
	file, err := fs.FileByID(doc.DocID)
	if err != nil {
		// File may have been deleted between event and now - not an error
		log.Warnf("trigger antivirus: Could not get file %s for pending status: %s", doc.DocID, err)
		return
	}

	newdoc := file.Clone().(*vfs.FileDoc)
	newdoc.AntivirusScan = &vfs.AntivirusScan{
		Status: vfs.AVStatusPending,
	}
	if err := couchdb.UpdateDoc(fs, newdoc); err != nil {
		// Conflict or other error - log and continue, job will still run
		log.Warnf("trigger antivirus: Could not set pending status for file %s: %s", doc.DocID, err)
	}
}

// Unschedule stops the trigger.
func (t *AntivirusTrigger) Unschedule() {
	close(t.unscheduled)
}
