package couchdb

import (
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/pkg/realtime"
)

// Hook is a function called before a change is made into
// A hook can block the event by returning an error
type listener func(db prefixer.Prefixer, doc Doc, old Doc) error

type key struct {
	DocType string
	Event   string
}

var hooks map[key][]listener

// Events to hook into
const (
	EventCreate = realtime.EventCreate
	EventUpdate = realtime.EventUpdate
	EventDelete = realtime.EventDelete
)

// Run runs all hooks for the given event.
func runHooks(db Database, event string, doc Doc, old Doc) error {
	if hs, ok := hooks[key{doc.DocType(), event}]; ok {
		for _, h := range hs {
			err := h(db, doc, old)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// AddHook adds an hook for the given doctype and event.
// Useful for special doctypes cleanup
func AddHook(doctype, event string, hook listener) {
	if hooks == nil {
		hooks = make(map[key][]listener)
	}
	k := key{doctype, event}
	hs, ok := hooks[k]
	if !ok {
		hs = make([]listener, 0)
	}
	hooks[k] = append(hs, hook)
}
