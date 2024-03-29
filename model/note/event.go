package note

import (
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/realtime"
)

// Event is a realtime event for a note (like the update of the position of a
// pointer).
type Event map[string]interface{}

// ID returns the event qualified identifier
func (e Event) ID() string {
	id, _ := e["_id"].(string)
	return id
}

// Rev returns the event revision
func (e Event) Rev() string {
	rev, _ := e["_rev"].(string)
	return rev
}

// DocType returns the document type
func (e Event) DocType() string { return consts.NotesEvents }

// Clone implements couchdb.Doc
func (e Event) Clone() couchdb.Doc {
	cloned := make(Event)
	for k, v := range e {
		cloned[k] = v
	}
	return cloned
}

// SetID changes the event qualified identifier
func (e Event) SetID(id string) {
	if id == "" {
		delete(e, "_id")
	} else {
		e["_id"] = id
	}
}

// SetRev changes the event revision
func (e Event) SetRev(rev string) {
	if rev == "" {
		delete(e, "_rev")
	} else {
		e["_rev"] = rev
	}
}

// Included is part of the jsonapi.Object interface
func (e Event) Included() []jsonapi.Object { return nil }

// Links is part of the jsonapi.Object interface
func (e Event) Links() *jsonapi.LinksList { return nil }

// Relationships is part of the jsonapi.Object interface
func (e Event) Relationships() jsonapi.RelationshipMap { return nil }

func (e Event) publish(inst *instance.Instance) {
	go realtime.GetHub().Publish(inst, realtime.EventUpdate, e, nil)
}

// PutTelepointer sends the position of a pointer in the realtime hub.
func PutTelepointer(inst *instance.Instance, t Event) error {
	if t["sessionID"] == nil || t["sessionID"] == "" {
		return ErrMissingSessionID
	}
	t["doctype"] = consts.NotesTelepointers
	t.publish(inst)
	return nil
}

func publishUpdatedTitle(inst *instance.Instance, fileID, title, sessionID string) {
	event := Event{
		"title":     title,
		"doctype":   consts.NotesDocuments,
		"sessionID": sessionID,
	}
	event.SetID(fileID)
	event.publish(inst)
}

func publishSteps(inst *instance.Instance, fileID string, steps []Step) {
	for _, s := range steps {
		e := Event(s)
		e["doctype"] = s.DocType()
		e.SetID(fileID)
		e.publish(inst)
	}
}

// PublishThumbnail sends information about a resized image.
func PublishThumbnail(inst *instance.Instance, event Event) {
	parts := strings.SplitN(event.ID(), "/", 2)
	event.SetID(parts[0])        // The note ID is the first part of the image ID
	event["image_id"] = parts[1] // The image ID is the second part
	event.publish(inst)
}

var _ jsonapi.Object = &Event{}
