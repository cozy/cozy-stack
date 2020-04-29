package sharing

import (
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

func init() {
	vfs.RevokeSharingFunc = revokeTrashed
}

const (
	SharingDirAlreadyTrashed = true
	SharingDirNotTrashed     = false
)

func revokeTrashed(db prefixer.Prefixer, sharingID string) {
	s, err := FindSharing(db, sharingID)
	if err != nil {
		return
	}

	// XXX: we simulate an instance from the information of the prefixer. It is
	// an hack, but I don't see a better way to do that for the moment. Maybe
	// it is something we can improve later. I hope it should have all the
	// fields that are used for revoking the sharing.
	inst := &instance.Instance{
		Prefix: db.DBPrefix(),
		Domain: db.DomainName(),
	}

	if s.Owner {
		err = s.Revoke(inst)
	} else {
		err = s.RevokeRecipientBySelf(inst, SharingDirAlreadyTrashed)
	}
	if err != nil {
		inst.Logger().WithField("nspace", "sharing").
			Errorf("revokeTrashed failed for sharing %s: %s", sharingID, err)
	}
}
