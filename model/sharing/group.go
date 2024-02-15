package sharing

import (
	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/couchdb"
)

// Group contains the information about a group of members of the sharing.
type Group struct {
	ID      string `json:"id,omitempty"` // Only present on the instance where the group was added
	Name    string `json:"name"`
	AddedBy int    `json:"addedBy"` // The index of the member who have added the group
	Removed bool   `json:"removed,omitempty"`
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
	}

	g := Group{ID: groupID, Name: group.Name(), AddedBy: 0}
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
