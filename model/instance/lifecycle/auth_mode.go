package lifecycle

import "github.com/cozy/cozy-stack/model/instance"

func UpdateAuthMode(inst *instance.Instance, authMode instance.AuthMode) error {
	if inst.AuthMode == authMode {
		return nil
	}
	inst.AuthMode = authMode
	return update(inst)
}
