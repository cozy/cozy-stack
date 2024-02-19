package sharing

import (
	"sort"

	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	multierror "github.com/hashicorp/go-multierror"
)

// Group contains the information about a group of members of the sharing.
type Group struct {
	ID       string `json:"id,omitempty"` // Only present on the instance where the group was added
	Name     string `json:"name"`
	AddedBy  int    `json:"addedBy"` // The index of the member who have added the group
	ReadOnly bool   `json:"read_only"`
	Removed  bool   `json:"removed,omitempty"`
}

// GroupMessage is used for jobs on the share-group worker.
type GroupMessage struct {
	ContactID     string   `json:"contact_id"`
	GroupsAdded   []string `json:"added"`
	GroupsRemoved []string `json:"removed"`
}

// AddGroup adds a group of contacts identified by its ID to the members of the
// sharing.
func (s *Sharing) AddGroup(inst *instance.Instance, groupID string, readOnly bool) error {
	group, err := contact.FindGroup(inst, groupID)
	if err != nil {
		return err
	}
	contacts, err := group.FindContacts(inst)
	if err != nil {
		return err
	}

	groupIndex := len(s.Groups)
	for _, contact := range contacts {
		m, err := buildMemberFromContact(contact, readOnly)
		if err != nil {
			return err
		}
		m.OnlyInGroups = true
		_, idx, err := s.addMember(inst, m)
		if err != nil {
			return err
		}
		s.Members[idx].Groups = append(s.Members[idx].Groups, groupIndex)
		sort.Ints(s.Members[idx].Groups)
	}

	g := Group{ID: groupID, Name: group.Name(), AddedBy: 0, ReadOnly: readOnly}
	s.Groups = append(s.Groups, g)
	return nil
}

// RevokeGroup revoke a group of members on the sharer Cozy. After that, the
// sharing is desactivated if there are no longer any active recipient.
func (s *Sharing) RevokeGroup(inst *instance.Instance, index int) error {
	if !s.Owner {
		return ErrInvalidSharing
	}

	for i, m := range s.Members {
		inGroup := false
		for _, idx := range m.Groups {
			if idx == index {
				inGroup = true
			}
		}
		if !inGroup {
			continue
		}
		if len(m.Groups) == 1 {
			s.Members[i].Groups = nil
		} else {
			var groups []int
			for _, idx := range m.Groups {
				if idx != index {
					groups = append(groups, idx)
				}
			}
			s.Members[i].Groups = groups
		}
		if m.OnlyInGroups && len(s.Members[i].Groups) == 0 {
			if err := s.RevokeRecipient(inst, i); err != nil {
				return err
			}
		}
	}

	s.Groups[index].Removed = true
	return couchdb.UpdateDoc(inst, s)
}

// UpdateGroups is called when a contact is added or removed to a group. It
// finds the sharings for this group, and adds or removes the member to those
// sharings.
func UpdateGroups(inst *instance.Instance, msg GroupMessage) error {
	sharings, err := FindActive(inst)
	if err != nil {
		return err
	}

	var errm error
	for _, s := range sharings {
		for _, added := range msg.GroupsAdded {
			for idx, group := range s.Groups {
				if group.ID == added {
					if err := s.AddMemberToGroup(inst, idx, msg.ContactID); err != nil {
						errm = multierror.Append(errm, err)
					}
				}
			}
		}
		for _, removed := range msg.GroupsRemoved {
			for idx, group := range s.Groups {
				if group.ID == removed {
					if err := s.RemoveMemberFromGroup(inst, idx, msg.ContactID); err != nil {
						errm = multierror.Append(errm, err)
					}
				}
			}
		}
	}

	return errm
}

// AddMemberToGroup adds a contact to a sharing via a group.
func (s *Sharing) AddMemberToGroup(inst *instance.Instance, groupIndex int, contactID string) error {
	contact, err := contact.Find(inst, contactID)
	if err != nil {
		return err
	}

	readOnly := s.Groups[groupIndex].ReadOnly
	m, err := buildMemberFromContact(contact, readOnly)
	if err != nil {
		return err
	}
	m.OnlyInGroups = true
	_, idx, err := s.addMember(inst, m)
	if err != nil {
		return err
	}
	s.Members[idx].Groups = append(s.Members[idx].Groups, groupIndex)
	sort.Ints(s.Members[idx].Groups)

	return couchdb.UpdateDoc(inst, s)
}

// RemoveMemberFromGroup removes a member of a group.
func (s *Sharing) RemoveMemberFromGroup(inst *instance.Instance, groupIndex int, contactID string) error {
	contact, err := contact.Find(inst, contactID)
	if err != nil {
		return err
	}
	var email string
	if addr, err := contact.ToMailAddress(); err == nil {
		email = addr.Email
	}
	cozyURL := contact.PrimaryCozyURL()

	matchMember := func(m Member) bool {
		if m.Email != "" && m.Email == email {
			return true
		}
		if m.Instance != "" && m.Instance == cozyURL {
			return true
		}
		return false
	}

	for i, m := range s.Members {
		if !matchMember(m) {
			continue
		}

		var groups []int
		for _, idx := range m.Groups {
			if idx != groupIndex {
				groups = append(groups, idx)
			}
		}
		s.Members[i].Groups = groups

		if m.OnlyInGroups && len(s.Members[i].Groups) == 0 {
			return s.RevokeRecipient(inst, i)
		} else {
			return couchdb.UpdateDoc(inst, s)
		}
	}

	return nil
}
