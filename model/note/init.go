package note

import "github.com/cozy/cozy-stack/model/instance"

var globalSvc *Note

func Init() *Note {
	svc := NewNote()
	globalSvc = svc

	return svc
}

// Update is a global wrapper around [Service.Update].
//
// Deprecated: Please use dependency injection instead.
func Update(inst *instance.Instance, fileID string) error {
	return globalSvc.Update(inst, fileID)
}

// FlushPendings is a global wrapper around [Service.FlushPendings].
//
// Deprecated: Please use dependency injection instead.
func FlushPendings(inst *instance.Instance) error {
	return globalSvc.FlushPendings(inst)
}
