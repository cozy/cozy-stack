package orgdrive

import (
	"errors"
	"strings"

	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/orgdirectory"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// The reasons a member or group is skipped.
const (
	ReasonNotInDirectory = "not in the organization directory"
	ReasonSeveralInDir   = "several contacts in the organization directory"
	ReasonDriveFull      = "the drive has too many members"
)

// syncMembers makes the members and groups of the drive match the command.
// The role of a member is write when their direct role or one of their groups
// gives it, and read otherwise. Everything is decided from the stored sharing,
// so a redelivery after a failure finishes the job, invitations included.
func syncMembers(inst *instance.Instance, s *sharing.Sharing, cmd *Command) ([]Skipped, error) {
	if err := revokeGroups(inst, s, cmd); err != nil {
		return nil, err
	}
	skipped, dirty, err := attachGroups(inst, s, cmd)
	if err != nil {
		return nil, err
	}

	wanted := make(map[string]bool, len(cmd.Members)) // email -> read only
	for _, m := range cmd.Members {
		wanted[strings.ToLower(m.Email)] = m.Role == RoleRead
	}
	present := make(map[string]bool, len(s.Members))
	for i := 1; i < len(s.Members); i++ {
		m := &s.Members[i]
		if m.Status == sharing.MemberStatusRevoked {
			continue
		}
		email := strings.ToLower(m.Email)
		readOnly, direct := wanted[email]
		present[email] = direct
		if !direct && len(m.Groups) == 0 {
			if err := s.RevokeRecipient(inst, i); err != nil {
				return nil, err
			}
			continue
		}
		if m.OnlyInGroups == direct {
			m.OnlyInGroups = !direct
			dirty = true
		}
		if err := setRole(inst, s, i, (!direct || readOnly) && !inWritableGroup(s, i)); err != nil {
			return nil, err
		}
	}

	for _, m := range cmd.Members {
		if present[strings.ToLower(m.Email)] {
			continue
		}
		c, err := orgdirectory.FindManagedContactByEmail(inst, m.Email)
		switch {
		case errors.Is(err, contact.ErrNotFound):
			skipped = append(skipped, Skipped{Email: m.Email, Reason: ReasonNotInDirectory})
			continue
		case errors.Is(err, orgdirectory.ErrMultipleManagedContacts):
			skipped = append(skipped, Skipped{Email: m.Email, Reason: ReasonSeveralInDir})
			continue
		case err != nil:
			return nil, err
		}
		err = s.AddContact(inst, c.ID(), m.Role == RoleRead)
		if errors.Is(err, sharing.ErrTooManyMembers) {
			skipped = append(skipped, Skipped{Email: m.Email, Reason: ReasonDriveFull})
			continue
		}
		if err != nil {
			return nil, err
		}
		dirty = true
	}

	if dirty {
		if err := couchdb.UpdateDoc(inst, s); err != nil {
			return nil, err
		}
	}
	if err := invite(inst, s); err != nil {
		return nil, err
	}
	return skipped, nil
}

// revokeGroups keeps a group whose role changed: its members get their new
// role from the role pass of syncMembers.
func revokeGroups(inst *instance.Instance, s *sharing.Sharing, cmd *Command) error {
	wanted := make(map[string]bool, len(cmd.Groups))
	for _, g := range cmd.Groups {
		wanted[groupDocID(cmd, g.ID)] = true
	}
	for i, g := range s.Groups {
		if g.Revoked || wanted[g.ID] {
			continue
		}
		if err := s.RevokeGroup(inst, i); err != nil {
			return err
		}
	}
	return nil
}

// attachGroups sets the role of the groups already on the drive and adds the
// missing ones.
func attachGroups(inst *instance.Instance, s *sharing.Sharing, cmd *Command) ([]Skipped, bool, error) {
	var skipped []Skipped
	dirty := false
	for _, g := range cmd.Groups {
		id := groupDocID(cmd, g.ID)
		readOnly := g.Role == RoleRead
		if i := groupIndex(s, id); i >= 0 {
			if s.Groups[i].ReadOnly != readOnly {
				s.Groups[i].ReadOnly = readOnly
				dirty = true
			}
			continue
		}
		// AttachGroup can fail after adding some of the group's members, so it
		// works on a copy that is kept only when the whole group is added.
		trial := s.Clone().(*sharing.Sharing)
		err := trial.AttachGroup(inst, id, readOnly)
		switch {
		case couchdb.IsNotFoundError(err) || couchdb.IsNoDatabaseError(err):
			skipped = append(skipped, Skipped{GroupID: g.ID, Reason: ReasonNotInDirectory})
		case errors.Is(err, sharing.ErrTooManyMembers):
			skipped = append(skipped, Skipped{GroupID: g.ID, Reason: ReasonDriveFull})
		case err != nil:
			return nil, false, err
		default:
			*s = *trial
			dirty = true
		}
	}
	return skipped, dirty, nil
}

func setRole(inst *instance.Instance, s *sharing.Sharing, i int, readOnly bool) error {
	if s.Members[i].ReadOnly == readOnly {
		return nil
	}
	if readOnly {
		return s.AddReadOnlyFlag(inst, i)
	}
	return s.RemoveReadOnlyFlag(inst, i)
}

func inWritableGroup(s *sharing.Sharing, i int) bool {
	for _, idx := range s.Members[i].Groups {
		if idx < len(s.Groups) && !s.Groups[idx].Revoked && !s.Groups[idx].ReadOnly {
			return true
		}
	}
	return false
}

func groupIndex(s *sharing.Sharing, id string) int {
	for i, g := range s.Groups {
		if g.ID == id && !g.Revoked {
			return i
		}
	}
	return -1
}

func groupDocID(cmd *Command, id string) string {
	return orgdirectory.GroupDocID(cmd.OrganizationID, id)
}

// invite sends the invitations not sent yet. It runs on every command, since
// those members can come from an earlier delivery that failed after writing
// them.
func invite(inst *instance.Instance, s *sharing.Sharing) error {
	pending := false
	for _, m := range s.Members[1:] {
		if m.Status == sharing.MemberStatusMailNotSent {
			pending = true
			break
		}
	}
	if !pending {
		return nil
	}
	err := s.SendInvitations(inst, nil)
	if errors.Is(err, sharing.ErrInvitationNotSent) {
		log.Warnf("Some invitations to the drive %s were not sent", s.SID)
	} else if err != nil {
		return err
	}
	go s.Clone().(*sharing.Sharing).NotifyRecipients(inst, nil)
	return nil
}
