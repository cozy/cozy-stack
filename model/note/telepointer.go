package note

import (
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/realtime"
)

// Telepointer is the position of a cursor inside a note.
type Telepointer map[string]interface{}

// ID returns the directory qualified identifier
func (t Telepointer) ID() string {
	id, _ := t["_id"].(string)
	return id
}

// Rev returns the directory revision
func (t Telepointer) Rev() string {
	rev, _ := t["_rev"].(string)
	return rev
}

// DocType returns the document type
func (t Telepointer) DocType() string { return consts.NotesTelepointers }

// Clone implements couchdb.Doc
func (t Telepointer) Clone() couchdb.Doc {
	cloned := make(Telepointer)
	for k, v := range t {
		cloned[k] = v
	}
	return cloned
}

// SetID changes the telepointer qualified identifier
func (t Telepointer) SetID(id string) {
	if id == "" {
		delete(t, "_id")
	} else {
		t["_id"] = id
	}
}

// SetRev changes the telepointer revision
func (t Telepointer) SetRev(rev string) {
	if rev == "" {
		delete(t, "_rev")
	} else {
		t["_rev"] = rev
	}
}

// Included is part of the jsonapi.Object interface
func (t Telepointer) Included() []jsonapi.Object { return nil }

// Links is part of the jsonapi.Object interface
func (t Telepointer) Links() *jsonapi.LinksList { return nil }

// Relationships is part of the jsonapi.Object interface
func (t Telepointer) Relationships() jsonapi.RelationshipMap { return nil }

// PutTelepointer sends the position of a pointer in the realtime hub.
func PutTelepointer(inst *instance.Instance, t Telepointer) error {
	if t.ID() == "" {
		return ErrMissingSessionID
	}
	go realtime.GetHub().Publish(inst, realtime.EventUpdate, t, nil)
	return nil
}

var _ jsonapi.Object = &Telepointer{}
