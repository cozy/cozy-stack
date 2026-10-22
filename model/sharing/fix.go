package sharing

import (
	"fmt"

	"github.com/cozy/cozy-stack/model/instance"
	multierror "github.com/hashicorp/go-multierror"
)

// sharingFixers are the fixers for the issues reported by CheckSharings,
// indexed by the type of the issue.
var sharingFixers = map[string]func(s *Sharing, inst *instance.Instance) error{
	// Do what the missed revocation notification would have done
	"drive_revoked_by_owner": (*Sharing).RevokeByNotification,
}

// FixSharings fixes the issues reported by CheckSharings that have a fixer.
// It returns the fixed issues. The files and folders consistency is not
// checked, as there is no fixer for it.
func FixSharings(inst *instance.Instance) ([]map[string]interface{}, error) {
	checks, err := CheckSharings(inst, true)
	if err != nil {
		return nil, err
	}

	var errm error
	fixed := []map[string]interface{}{}
	for _, check := range checks {
		typ, _ := check["type"].(string)
		fixer, ok := sharingFixers[typ]
		if !ok {
			continue
		}
		sid, _ := check["id"].(string)
		s, err := FindSharing(inst, sid)
		if err == nil {
			err = fixer(s, inst)
		}
		if err != nil {
			errm = multierror.Append(errm, fmt.Errorf("cannot fix %s for sharing %s: %w", typ, sid, err))
			continue
		}
		inst.Logger().WithNamespace("sharing").Infof("Fixed %s for sharing %s", typ, sid)
		fixed = append(fixed, check)
	}
	return fixed, errm
}
