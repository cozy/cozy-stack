package note

import (
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
)

var globalSvc *Note

func Init() *Note {
	svc := Note{
		lock: config.Lock(),
	}

	globalSvc = &svc

	return &svc
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
