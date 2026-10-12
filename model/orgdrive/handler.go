package orgdrive

import (
	"context"
	"fmt"

	"github.com/cozy/cozy-stack/model/orgdirectory"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/messaging"
	"github.com/cozy/cozy-stack/pkg/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
)

func init() {
	rabbitmq.RegisterHandler(rabbitmq.QueueDriveCommands, func(p messaging.Publisher) rabbitmq.Handler {
		return NewHandler(p)
	})
}

var log = logger.WithNamespace("orgdrive")

// ReadyData is the data of com.twake.drive.org_drive.ready.v1.
type ReadyData struct {
	Reference Reference `json:"reference"`
	Revision  int64     `json:"revision"`
	Drive     DriveRef  `json:"drive"`
	Skipped   []Skipped `json:"skipped"`
}

// DriveRef is the id of the drive sharing, the same on every member's
// instance.
type DriveRef struct {
	ID string `json:"id"`
}

// Skipped is a member or group that could not be put on the drive.
type Skipped struct {
	Email   string `json:"email,omitempty"`
	GroupID string `json:"group_id,omitempty"`
	Reason  string `json:"reason"`
}

// Handler applies the drive.org_drive.ensure commands.
type Handler struct {
	publisher messaging.Publisher
}

// NewHandler returns a handler replying with this publisher.
func NewHandler(publisher messaging.Publisher) *Handler {
	return &Handler{publisher: publisher}
}

// Handle applies one command and publishes its reply.
func (h *Handler) Handle(ctx context.Context, d amqp.Delivery) error {
	if d.RoutingKey != rabbitmq.RoutingKeyOrgDriveEnsure {
		return fmt.Errorf("orgdrive: %w: routing key %s", rabbitmq.ErrMalformed, d.RoutingKey)
	}
	cmd, err := ParseCommand(d.Body)
	if err != nil {
		return err
	}
	if cmd.State == StateRemoved {
		return malformed("state removed is not supported yet")
	}

	inst, err := orgdirectory.FindOrganizationInstance(ctx, cmd.OrganizationID, "")
	if err != nil {
		return fmt.Errorf("%s: %w", d.RoutingKey, err)
	}
	if inst == nil {
		return fmt.Errorf("%s: no organization instance for %s", d.RoutingKey, cmd.OrganizationID)
	}

	drive, skipped, err := Ensure(inst, cmd)
	if err != nil {
		return fmt.Errorf("%s: %s/%s on %s: %w", d.RoutingKey, cmd.Reference.Source, cmd.Reference.ID, inst.Domain, err)
	}
	if drive == nil {
		log.Infof("%s/%s: revision %d is older than the drive, ignored", cmd.Reference.Source, cmd.Reference.ID, cmd.Revision)
		return nil
	}

	req, err := rabbitmq.NewActivityRequest(inst.ContextName, rabbitmq.RoutingKeyOrgDriveReady, cmd.OrganizationID, ReadyData{
		Reference: cmd.Reference,
		Revision:  cmd.Revision,
		Drive:     DriveRef{ID: drive.SID},
		Skipped:   append([]Skipped{}, skipped...),
	})
	if err != nil {
		return err
	}
	if err := h.publisher.Publish(ctx, req); err != nil {
		return fmt.Errorf("%s: reply for %s/%s: %w", d.RoutingKey, cmd.Reference.Source, cmd.Reference.ID, err)
	}
	log.Infof("%s/%s: drive %s ready on %s at revision %d", cmd.Reference.Source, cmd.Reference.ID, drive.SID, inst.Domain, cmd.Revision)

	if !drive.Reference.Ready {
		drive.Reference.Ready = true
		if err := couchdb.UpdateDoc(inst, drive); err != nil {
			return fmt.Errorf("%s: mark drive %s ready: %w", d.RoutingKey, drive.SID, err)
		}
	}
	return nil
}
