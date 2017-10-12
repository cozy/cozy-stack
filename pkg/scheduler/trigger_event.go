package scheduler

import (
	"errors"
	"strings"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/pkg/vfs"
)

// EventTrigger implements Trigger for realtime triggered events
type EventTrigger struct {
	unscheduled chan struct{}
	infos       *TriggerInfos
	mask        permissions.Rule
}

// NewEventTrigger returns a new instance of EventTrigger given the specified
// options.
func NewEventTrigger(infos *TriggerInfos) (*EventTrigger, error) {
	rule, err := permissions.UnmarshalRuleString(infos.Arguments)
	if err != nil {
		return nil, err
	}
	return &EventTrigger{
		unscheduled: make(chan struct{}),
		infos:       infos,
		mask:        rule,
	}, nil
}

// Type implements the Type method of the Trigger interface.
func (t *EventTrigger) Type() string {
	return t.infos.Type
}

// DocType implements the permissions.Validable interface
func (t *EventTrigger) DocType() string {
	return consts.Triggers
}

// ID implements the permissions.Validable interface
func (t *EventTrigger) ID() string {
	return t.infos.TID
}

// Valid implements the permissions.Validable interface
func (t *EventTrigger) Valid(key, value string) bool {
	switch key {
	case jobs.WorkerType:
		return t.infos.WorkerType == value
	}
	return false
}

// Schedule implements the Schedule method of the Trigger interface.
func (t *EventTrigger) Schedule() <-chan *jobs.JobRequest {
	ch := make(chan *jobs.JobRequest)
	go func() {
		sub := realtime.GetHub().Subscriber(t.infos.Domain)
		sub.Subscribe(t.mask.Type)
		defer func() {
			sub.Close()
			close(ch)
		}()
		for {
			select {
			case e := <-sub.Channel:
				if eventMatchPermission(e, &t.mask) {
					ch <- t.Infos().JobRequestWithEvent(e)
				}
			case <-t.unscheduled:
				return
			}
		}
	}()
	return ch
}

// Unschedule implements the Unschedule method of the Trigger interface.
func (t *EventTrigger) Unschedule() {
	close(t.unscheduled)
}

// Infos implements the Infos method of the Trigger interface.
func (t *EventTrigger) Infos() *TriggerInfos {
	return t.infos
}

func eventMatchPermission(e *realtime.Event, rule *permissions.Rule) bool {
	if e.Doc.DocType() != rule.Type {
		return false
	}

	if !rule.Verbs.Contains(permissions.Verb(e.Verb)) {
		return false
	}

	if len(rule.Values) == 0 {
		return true
	}

	if rule.Selector == "" {
		if rule.ValuesContain(e.Doc.ID()) {
			return true
		}
		if e.Doc.DocType() == consts.Files {

			for _, value := range rule.Values {
				var dir vfs.DirDoc
				db := couchdb.SimpleDatabasePrefix(e.Domain)
				if err := couchdb.GetDoc(db, consts.Files, value, &dir); err != nil {
					logger.WithNamespace("event-trigger").Error(err)
					return false
				}
				if testPath(&dir, e.Doc) {
					return true
				}
				if e.OldDoc != nil {
					if testPath(&dir, e.OldDoc) {
						return true
					}
				}
			}
		}
		return false
	}

	if v, ok := e.Doc.(permissions.Validable); ok {
		if rule.ValuesValid(v) {
			return true
		}
		// Particular case where the new doc is not valid but the old one was.
		if e.OldDoc != nil {
			if vOld, okOld := e.OldDoc.(permissions.Validable); okOld {
				return rule.ValuesValid(vOld)
			}
		}
	}

	return false
}

// DumpFilePather is a struct made for calling the Path method of a FileDoc and
// relying on the cached fullpath of this document (not trying to rebuild it)
type DumpFilePather struct{}

// FilePath only returns an error saying to not call this method
func (d DumpFilePather) FilePath(doc *vfs.FileDoc) (string, error) {
	logger.WithNamespace("event-trigger").Warning("FilePath method of DumpFilePather has been called")
	return "", errors.New("DumpFilePather FilePath should not have been called")
}

var dumpFilePather = DumpFilePather{}

func testPath(dir *vfs.DirDoc, doc realtime.Doc) bool {
	if d, ok := doc.(*vfs.DirDoc); ok {
		return strings.HasPrefix(d.Fullpath, dir.Fullpath+"/")
	}
	if f, ok := doc.(*vfs.FileDoc); ok {
		if f.Trashed {
			return strings.HasPrefix(f.RestorePath, dir.Fullpath)
		}
		p, err := f.Path(dumpFilePather)
		if err != nil {
			return false
		}
		return strings.HasPrefix(p, dir.Fullpath+"/")
	}
	return false
}
