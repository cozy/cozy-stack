package sharing

import (
	"errors"
	"net/url"

	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// isDriveRevokedByOwner tells if the owner of a shared drive no longer
// recognizes this member, while the sharing is still active on the member
// instance. It happens when the owner has revoked the member, but the
// revocation notification has never reached the member instance. Only an
// owner on the same stack can be checked: false is returned otherwise.
func (s *Sharing) isDriveRevokedByOwner() (bool, error) {
	if !s.Drive || s.Owner || !s.Active || len(s.Members) == 0 ||
		len(s.Credentials) != 1 || s.Credentials[0].DriveToken == "" {
		return false, nil
	}
	u, err := url.Parse(s.Members[0].Instance)
	if err != nil {
		return false, nil
	}
	owner := LocalInstanceFromHost(u.Host)
	if owner == nil {
		return false, nil
	}

	ownerSharing, err := FindSharing(owner, s.SID)
	if couchdb.IsNotFoundError(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if !ownerSharing.Active {
		return true, nil
	}

	member, err := ownerSharing.FindMemberByInteractCode(owner, s.Credentials[0].DriveToken)
	if errors.Is(err, ErrMemberNotFound) || couchdb.IsNotFoundError(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return member.Status == MemberStatusRevoked, nil
}

func (s *Sharing) checkDriveRevokedByOwner() ([]map[string]interface{}, error) {
	revoked, err := s.isDriveRevokedByOwner()
	if err != nil || !revoked {
		return nil, err
	}
	return []map[string]interface{}{{
		"id":    s.SID,
		"type":  "drive_revoked_by_owner",
		"owner": s.Members[0].Instance,
	}}, nil
}
