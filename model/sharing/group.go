package sharing

import (
	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/model/instance"
)

// Groups contains the information about a group of contacts that have been
// added as recipient to a sharing.
type Group struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	On   string `json:"on"` // The instance where the group has been added
}

// AddGroup add the group and its contacts to the sharing.
func (s *Sharing) AddGroup(inst *instance.Instance, groupID string, readOnly bool) error {
	for _, g := range s.Groups {
		if g.ID == groupID {
			return nil
		}
	}

	group, err := contact.FindGroup(inst, groupID)
	if err != nil {
		return err
	}
	instanceURL := inst.PageURL("/", nil)
	name := group.Name()
	s.Groups = append(s.Groups, Group{ID: groupID, Name: name, On: instanceURL})
	contacts, err := group.ListContacts(inst)
	if err != nil {
		return err
	}
	for _, c := range contacts {
		var name, email string
		cozyURL := c.PrimaryCozyURL()
		addr, err := c.ToMailAddress()
		if err == nil {
			name = addr.Name
			email = addr.Email
		} else {
			if cozyURL == "" {
				return err
			}
			name = c.PrimaryName()
		}
		m := Member{
			Status:     MemberStatusMailNotSent,
			Name:       name,
			Email:      email,
			Instance:   cozyURL,
			ReadOnly:   readOnly,
			GroupsOnly: true,
			Groups:     []string{groupID},
		}
		if _, err := s.addMember(inst, m); err != nil {
			return err
		}
	}
	return nil
}
