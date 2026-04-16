package nextcloud

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/rabbitmq"
	"github.com/cozy/cozy-stack/pkg/webdav"
)

// migrationTriggerLockName is the per-instance lock held across the
// re-check, account upsert, tracking-doc insert, and RabbitMQ publish.
// Without it, two concurrent triggers could both see no active migration
// and both commit a pending tracking document, breaking the
// one-migration-per-instance invariant.
const migrationTriggerLockName = "nextcloud/migration-trigger"

// ErrNextcloudUnreachable wraps any error surfaced while probing the
// Nextcloud server other than an explicit auth rejection (401/403). The
// caller should translate it to a 502 Bad Gateway for the HTTP client.
var ErrNextcloudUnreachable = errors.New("nextcloud unreachable")

// ErrMigrationBrokerUnavailable is returned when the RabbitMQ publish
// fails after the tracking document has been created. The tracking
// document is marked failed before this error is returned, so retries
// are not blocked by a stuck pending doc.
var ErrMigrationBrokerUnavailable = errors.New("migration broker unavailable")

// TriggerMigrationRequest carries the user-supplied inputs needed to
// start a bulk Nextcloud-to-Cozy migration. Field semantics match the
// HTTP request body on POST /remote/nextcloud/migration.
type TriggerMigrationRequest struct {
	NextcloudURL         string
	NextcloudLogin       string
	NextcloudAppPassword string
	SourcePath           string
	// TargetDir is the absolute Cozy path under which the migration service
	// writes imported files. Empty means "use DefaultMigrationTargetDir".
	TargetDir string
}

// TriggerMigration is the single entry point for "start a Nextcloud
// migration for this instance with these credentials". It probes the
// remote host, serializes itself against concurrent triggers via a
// per-instance lock, upserts the io.cozy.accounts document, creates a
// pending tracking document, and publishes the migration command to
// RabbitMQ. On success it returns the created tracking document.
//
// Error contract, in priority order, so callers can map them to HTTP
// status codes via errors.Is:
//
//   - [ErrMigrationConflict]: a pending or running migration already
//     exists for this instance.
//   - [webdav.ErrInvalidAuth]: the Nextcloud host rejected the supplied
//     credentials with 401/403.
//   - [ErrNextcloudUnreachable]: the credentials probe failed for any
//     other reason (DNS, TLS, unexpected status, decode error).
//   - [ErrMigrationBrokerUnavailable]: the RabbitMQ publish failed and
//     the tracking document was marked failed.
//   - any other error: treat as an internal server failure.
func TriggerMigration(
	ctx context.Context,
	inst *instance.Instance,
	req TriggerMigrationRequest,
	rmq rabbitmq.Service,
	log logger.Logger,
) (*Migration, error) {
	// Cheap pre-check outside the lock: avoid a probe round trip when a
	// migration is already in flight. The authoritative check happens
	// inside the lock below.
	if active, err := FindActiveMigration(inst); err != nil {
		log.Errorf("Failed to query active migrations: %s", err)
		return nil, fmt.Errorf("find active migration: %w", err)
	} else if active != nil {
		log.WithField("active_migration_id", active.ID()).
			Infof("Rejecting new Nextcloud migration: one is already in flight")
		return nil, ErrMigrationConflict
	}

	// Probe outside the lock: the network call can take seconds and must
	// not serialize unrelated triggers. Attach the request-scoped logger
	// to ctx so the probe can surface its diagnostics under the same
	// instance/migration fields the handler already tagged.
	probeCtx := logger.WithContext(ctx, log)
	userID, err := FetchUserIDWithCredentials(probeCtx, req.NextcloudURL, req.NextcloudLogin, req.NextcloudAppPassword)
	if err != nil {
		if errors.Is(err, webdav.ErrInvalidAuth) {
			log.Infof("Nextcloud credentials probe rejected by remote host")
			return nil, err
		}
		log.Warnf("Nextcloud credentials probe failed: %s", err)
		return nil, fmt.Errorf("%w: %w", ErrNextcloudUnreachable, err)
	}

	mutex := config.Lock().ReadWrite(inst, migrationTriggerLockName)
	if err := mutex.Lock(); err != nil {
		log.Errorf("Failed to acquire migration trigger lock: %s", err)
		return nil, fmt.Errorf("acquire migration lock: %w", err)
	}
	defer mutex.Unlock()

	// Authoritative re-check under the lock: a racer may have inserted a
	// pending document between the cheap pre-check above and this line.
	if active, err := FindActiveMigration(inst); err != nil {
		log.Errorf("Failed to query active migrations: %s", err)
		return nil, fmt.Errorf("find active migration: %w", err)
	} else if active != nil {
		log.WithField("active_migration_id", active.ID()).
			Infof("Lost migration trigger race to a concurrent request")
		return nil, ErrMigrationConflict
	}

	accountID, err := EnsureAccount(inst, req.NextcloudURL, req.NextcloudLogin, req.NextcloudAppPassword, userID)
	if err != nil {
		log.Errorf("Failed to ensure nextcloud account: %s", err)
		return nil, fmt.Errorf("ensure nextcloud account: %w", err)
	}

	doc := NewPendingMigration(req.TargetDir)
	if err := couchdb.CreateDoc(inst, doc); err != nil {
		log.WithField("account_id", accountID).
			Errorf("Failed to create migration tracking doc: %s", err)
		return nil, fmt.Errorf("create migration tracking doc: %w", err)
	}
	triggerLogger := log.WithFields(logger.Fields{
		"migration_id": doc.DocID,
		"account_id":   accountID,
	})

	msg := rabbitmq.NextcloudMigrationRequestedMessage{
		MigrationID:   doc.DocID,
		WorkplaceFqdn: inst.Domain,
		AccountID:     accountID,
		SourcePath:    req.SourcePath,
		Timestamp:     time.Now().Unix(),
	}
	if err := rmq.Publish(ctx, rabbitmq.PublishRequest{
		ContextName: inst.ContextName,
		Exchange:    rabbitmq.ExchangeMigration,
		RoutingKey:  rabbitmq.RoutingKeyNextcloudMigrationRequested,
		Payload:     msg,
		MessageID:   doc.DocID,
	}); err != nil {
		triggerLogger.Errorf("Failed to publish migration command: %s", err)
		if markErr := doc.MarkFailed(inst, fmt.Errorf("publish migration command: %w", err)); markErr != nil {
			triggerLogger.Errorf("Failed to mark migration as failed after publish error: %s", markErr)
		}
		return nil, fmt.Errorf("%w: %w", ErrMigrationBrokerUnavailable, err)
	}

	triggerLogger.WithField("source_path", req.SourcePath).
		Infof("Nextcloud migration triggered")
	return doc, nil
}
