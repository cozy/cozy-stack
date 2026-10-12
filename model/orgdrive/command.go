package orgdrive

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cozy/cozy-stack/pkg/rabbitmq"
)

// The states a command can ask for.
const (
	StateCreated = "created"
	StateRemoved = "removed"
)

// The roles of a member or group on a drive.
const (
	RoleRead  = "read"
	RoleWrite = "write"
)

// Command is the payload of drive.org_drive.ensure: what the drive of a
// reference should look like.
type Command struct {
	OrganizationID string    `json:"organizationId"`
	Reference      Reference `json:"reference"`
	Revision       int64     `json:"revision"`
	State          string    `json:"state"`
	Name           string    `json:"name"`
	Members        []Member  `json:"members"`
	Groups         []Group   `json:"groups"`
	Activity       bool      `json:"activity"`
}

// Reference names the object of the app the drive is for.
type Reference struct {
	Source string `json:"source"`
	ID     string `json:"id"`
}

// Member is a person on the drive, found by email.
type Member struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// Group is a group of the organization on the drive.
type Group struct {
	ID   string `json:"id"`
	Role string `json:"role"`
}

// ParseCommand decodes and validates a command. Its errors wrap
// rabbitmq.ErrMalformed, since a redelivery cannot fix them.
func ParseCommand(body []byte) (*Command, error) {
	var cmd Command
	if err := json.Unmarshal(body, &cmd); err != nil {
		return nil, malformed("%s", err)
	}
	cmd.OrganizationID = strings.TrimSpace(cmd.OrganizationID)
	cmd.Reference.Source = strings.TrimSpace(cmd.Reference.Source)
	cmd.Reference.ID = strings.TrimSpace(cmd.Reference.ID)
	switch {
	case cmd.OrganizationID == "":
		return nil, malformed("missing organizationId")
	case cmd.Reference.Source == "" || cmd.Reference.ID == "":
		return nil, malformed("missing reference")
	case cmd.Revision <= 0:
		return nil, malformed("revision must be positive")
	case cmd.State != StateCreated && cmd.State != StateRemoved:
		return nil, malformed("unknown state %q", cmd.State)
	case cmd.State == StateCreated && strings.TrimSpace(cmd.Name) == "":
		return nil, malformed("missing name")
	}
	for i, m := range cmd.Members {
		cmd.Members[i].Email = strings.TrimSpace(m.Email)
		if cmd.Members[i].Email == "" {
			return nil, malformed("member without email")
		}
		if !validRole(m.Role) {
			return nil, malformed("unknown role %q for %s", m.Role, m.Email)
		}
	}
	for _, g := range cmd.Groups {
		if strings.TrimSpace(g.ID) == "" {
			return nil, malformed("group without id")
		}
		if !validRole(g.Role) {
			return nil, malformed("unknown role %q for group %s", g.Role, g.ID)
		}
	}
	return &cmd, nil
}

func validRole(role string) bool {
	return role == RoleRead || role == RoleWrite
}

func malformed(format string, args ...any) error {
	return fmt.Errorf("%s: %w: %s", rabbitmq.RoutingKeyOrgDriveEnsure, rabbitmq.ErrMalformed, fmt.Sprintf(format, args...))
}
