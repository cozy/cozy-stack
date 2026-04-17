package nextcloud

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/rabbitmq"
)

// ErrMigrationNotFound is returned when the tracking document referenced
// by a cancel request does not exist. Callers map it to a 404 rather
// than publishing into the void.
var ErrMigrationNotFound = errors.New("nextcloud migration not found")

// ErrMigrationAlreadyTerminal is returned when cancel is requested on a
// migration that has already reached completed, failed, or canceled.
// Callers map it to a 409. The migration service's cancel_requested
// fallback would swallow a stray message, but publishing when there is
// nothing to stop wastes broker work and pollutes metrics.
var ErrMigrationAlreadyTerminal = errors.New("nextcloud migration already in a terminal state")

// CancelMigration validates the request and publishes a cancel command.
// It intentionally does NOT mutate the tracking document: the terminal
// state transition is owned by the migration service so there is a single
// writer for it.
//
// The diagnostic logger is pulled from ctx via logger.FromContext so the
// caller can attach its request-scoped fields (migration_id, etc.) once
// with logger.WithContext rather than threading an extra parameter.
//
// Error contract, in priority order, so callers can map via errors.Is:
//
//   - [ErrMigrationNotFound]: no tracking doc with this id on the instance.
//   - [ErrMigrationAlreadyTerminal]: the migration has already reached a
//     terminal state (completed, failed, or canceled).
//   - [ErrMigrationBrokerUnavailable]: the RabbitMQ publish failed.
//     Unlike trigger, the tracking doc is NOT marked failed: a cancel
//     publish failure does not invalidate a migration that is already
//     running. The user retries, or the migration finishes normally.
//   - any other error: treat as an internal server failure.
func CancelMigration(
	ctx context.Context,
	inst *instance.Instance,
	migrationID string,
	rmq rabbitmq.Service,
) error {
	log := logger.FromContext(ctx)

	var doc Migration
	if err := couchdb.GetDoc(inst, consts.NextcloudMigrations, migrationID, &doc); err != nil {
		if couchdb.IsNotFoundError(err) || couchdb.IsNoDatabaseError(err) {
			return ErrMigrationNotFound
		}
		return fmt.Errorf("load migration %s: %w", migrationID, err)
	}
	if doc.IsTerminal() {
		log.WithField("status", doc.Status).
			Infof("Cancel rejected on terminal migration")
		return ErrMigrationAlreadyTerminal
	}

	msg := rabbitmq.NextcloudMigrationCanceledMessage{
		MigrationID:   migrationID,
		WorkplaceFqdn: inst.Domain,
		Timestamp:     time.Now().Unix(),
	}
	if err := rmq.Publish(ctx, rabbitmq.PublishRequest{
		ContextName: inst.ContextName,
		Exchange:    rabbitmq.ExchangeMigration,
		RoutingKey:  rabbitmq.RoutingKeyNextcloudMigrationCanceled,
		Payload:     msg,
		MessageID:   migrationID,
	}); err != nil {
		log.Errorf("Failed to publish migration cancel: %s", err)
		return fmt.Errorf("%w: %w", ErrMigrationBrokerUnavailable, err)
	}

	log.Infof("Nextcloud migration cancel requested")
	return nil
}
