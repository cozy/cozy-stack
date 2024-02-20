package job

import (
	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/realtime"
)

type ShareGroupTrigger struct {
	broker      Broker
	log         *logger.Entry
	unscheduled chan struct{}
}

// ShareGroupMessage is used for jobs on the share-group worker.
type ShareGroupMessage struct {
	ContactID       string   `json:"contact_id"`
	GroupsAdded     []string `json:"added"`
	GroupsRemoved   []string `json:"removed"`
	BecomeInvitable bool     `json:"invitable"`
}

func NewShareGroupTrigger(broker Broker) *ShareGroupTrigger {
	return &ShareGroupTrigger{
		broker:      broker,
		log:         logger.WithNamespace("scheduler"),
		unscheduled: make(chan struct{}),
	}
}

func (t *ShareGroupTrigger) Schedule() {
	sub := realtime.GetHub().SubscribeFirehose()
	defer sub.Close()
	for {
		select {
		case e := <-sub.Channel:
			if msg := t.match(e); msg != nil {
				t.pushJob(e, msg)
			}
		case <-t.unscheduled:
			return
		}
	}
}

func (t *ShareGroupTrigger) match(e *realtime.Event) *ShareGroupMessage {
	if e.Doc.DocType() != consts.Contacts {
		return nil
	}
	if e.Verb == realtime.EventNotify {
		return nil
	}

	newdoc, ok := e.Doc.(*couchdb.JSONDoc)
	if !ok {
		return nil
	}
	newContact := &contact.Contact{JSONDoc: *newdoc}
	newgroups := newContact.GroupIDs()

	var oldgroups []string
	invitable := false
	if olddoc, ok := e.OldDoc.(*couchdb.JSONDoc); ok {
		oldContact := &contact.Contact{JSONDoc: *olddoc}
		oldgroups = oldContact.GroupIDs()
		invitable = contactIsNowInvitable(oldContact, newContact)
	}

	added := diffGroupIDs(newgroups, oldgroups)
	removed := diffGroupIDs(oldgroups, newgroups)

	if len(added) == 0 && len(removed) == 0 && !invitable {
		return nil
	}

	return &ShareGroupMessage{
		ContactID:       e.Doc.ID(),
		GroupsAdded:     added,
		GroupsRemoved:   removed,
		BecomeInvitable: invitable,
	}
}

func diffGroupIDs(as, bs []string) []string {
	var diff []string
	for _, a := range as {
		found := false
		for _, b := range bs {
			if a == b {
				found = true
			}
		}
		if !found {
			diff = append(diff, a)
		}
	}
	return diff
}

func contactIsNowInvitable(oldContact, newContact *contact.Contact) bool {
	if oldURL := oldContact.PrimaryCozyURL(); oldURL != "" {
		return false
	}
	if oldAddr, err := oldContact.ToMailAddress(); err == nil && oldAddr.Email != "" {
		return false
	}
	if newURL := newContact.PrimaryCozyURL(); newURL != "" {
		return true
	}
	if newAddr, err := newContact.ToMailAddress(); err == nil && newAddr.Email != "" {
		return true
	}
	return false
}

func (t *ShareGroupTrigger) pushJob(e *realtime.Event, msg *ShareGroupMessage) {
	log := t.log.WithField("domain", e.Domain)
	m, err := NewMessage(msg)
	if err != nil {
		log.Infof("trigger share-group: cannot serialize message: %s", err)
		return
	}
	req := &JobRequest{
		WorkerType: "share-group",
		Message:    m,
	}
	log.Infof("trigger share-group: Pushing new job for contact %s", msg.ContactID)
	if _, err := t.broker.PushJob(e, req); err != nil {
		log.Errorf("trigger share-group: Could not schedule a new job: %s", err.Error())
	}
}

func (t *ShareGroupTrigger) Unschedule() {
	close(t.unscheduled)
}
