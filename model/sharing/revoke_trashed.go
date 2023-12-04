// Package sharing is where all the magic happen when documents/files are
// shared between several Cozy instances, from managing the recipients to
// replicating the changes.
package sharing

import (
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
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

	log := inst.Logger().WithNamespace("sharing")
	log.Infof("revokeTrashed called for sharing %s", sharingID)
	if s.Owner {
		err = s.Revoke(inst)
	} else {
		err = s.RevokeRecipientBySelf(inst, SharingDirAlreadyTrashed)
	}
	if err != nil {
		log.Errorf("revokeTrashed failed for sharing %s: %s", sharingID, err)
	}
}

// RevokeCipherSharings revoke all the sharings with the bitwarden ciphers.
func RevokeCipherSharings(inst *instance.Instance) error {
	sharings, err := GetSharingsByDocType(inst, consts.BitwardenCiphers)
	if err != nil {
		return err
	}
	for _, s := range sharings {
		if s.Owner {
			err = s.Revoke(inst)
		} else {
			err = s.RevokeRecipientBySelf(inst, SharingDirNotTrashed)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
