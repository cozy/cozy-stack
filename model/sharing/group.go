package sharing

import (
	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/model/instance"
)

// Group contains the information about a group of members of the sharing.
type Group struct {
	ID      string `json:"id,omitempty"` // Only present on the instance where the group was added
	Name    string `json:"name"`
	AddedBy int    `json:"addedBy"` // The index of the member who have added the group
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
