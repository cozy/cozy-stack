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

// syncMembers reconciles membership using the same sharing methods as the API.
// Redelivery finishes partially applied commands, including invitations.
func syncMembers(inst *instance.Instance, s *sharing.Sharing, cmd *Command) ([]Skipped, error) {
	if err := revokeGroups(inst, s, cmd); err != nil {
		return nil, err
	}
	var skipped []Skipped
	dirty := false

	wanted := make(map[string]bool, len(cmd.Members))
	for _, m := range cmd.Members {
		wanted[strings.ToLower(m.Email)] = true
	}
	present := make(map[string]int, len(s.Members))
	for i := 1; i < len(s.Members); i++ {
		if s.Members[i].Status != sharing.MemberStatusRevoked {
			present[strings.ToLower(s.Members[i].Email)] = i
		}
	}

	for _, m := range cmd.Members {
		i, exists := present[strings.ToLower(m.Email)]
		if exists && !s.Members[i].OnlyInGroups {
			if err := setRole(inst, s, i, m.Role == RoleRead); err != nil {
				return nil, err
			}
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
		if exists {
			s.Members[i].OnlyInGroups = false
		}
		dirty = true
	}

	groupSkipped, groupsChanged, err := attachGroups(inst, s, cmd)
	if err != nil {
		return nil, err
	}
	skipped = append(skipped, groupSkipped...)
	for i := 1; i < len(s.Members); i++ {
		m := &s.Members[i]
		if m.Status == sharing.MemberStatusRevoked || wanted[strings.ToLower(m.Email)] {
			continue
		}
		if len(m.Groups) == 0 {
			if err := s.RevokeRecipient(inst, i); err != nil {
				return nil, err
			}
		} else if !m.OnlyInGroups {
			m.OnlyInGroups = true
			dirty = true
		}
	}
	if dirty || groupsChanged {
		if err := couchdb.UpdateDoc(inst, s); err != nil {
			return nil, err
		}
	}
	if err := invite(inst, s); err != nil {
		return nil, err
	}
	return skipped, nil
}

// revokeGroups removes the groups no longer listed in the command.
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

// attachGroups adds groups with the model's existing role-conflict checks.
func attachGroups(inst *instance.Instance, s *sharing.Sharing, cmd *Command) ([]Skipped, bool, error) {
	var skipped []Skipped
	dirty := false
	for _, g := range cmd.Groups {
		id := groupDocID(cmd, g.ID)
		readOnly := g.Role == RoleRead
		if i := groupIndex(s, id); i >= 0 && s.Groups[i].ReadOnly == readOnly {
			continue
		}
		// AddGroup can fail after adding some of the group's members, so it
		// works on a copy that is kept only when the whole group is added.
		trial := s.Clone().(*sharing.Sharing)
		err := trial.AddGroup(inst, id, readOnly)
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
	if err := s.SendInvitations(inst, nil); err != nil {
		return err
	}
	go s.Clone().(*sharing.Sharing).NotifyRecipients(inst, nil)
	return nil
}
