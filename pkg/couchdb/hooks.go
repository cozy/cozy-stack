package couchdb

import "github.com/cozy/cozy-stack/pkg/realtime"

// Hook is a function called before a change is made into
// A hook can block the event by returning an error
type listener func(domain string, doc Doc, old Doc) error

var hooks map[string]map[string][]listener

// Events to hook into
const (
	EventCreate = realtime.EventCreate
	EventUpdate = realtime.EventUpdate
	EventDelete = realtime.EventDelete
)

// Run runs all hooks for the given event.
func runHooks(domain, event string, doc Doc, old Doc) error {
	if m, ok := hooks[doc.DocType()]; ok {
		if hs, ok := m[event]; ok {
			for _, h := range hs {
				err := h(domain, doc, old)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// AddHook adds an hook for the given doctype and event.
// Useful for special doctypes cleanup
func AddHook(doctype, event string, hook listener) {
	if hooks == nil {
		hooks = make(map[string]map[string][]listener)
	}
	m, ok := hooks[doctype]
	if !ok {
		m = make(map[string][]listener)
		hooks[doctype] = m
	}
	hs, ok := m[event]
	if !ok {
		hs = make([]listener, 0)
		m[event] = hs
	}
	m[event] = append(hs, hook)

}
