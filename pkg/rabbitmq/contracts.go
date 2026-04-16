package rabbitmq

const (
	ExchangeAuth      = "auth"
	ExchangeMigration = "migration"
)

const (
	QueueUserPasswordUpdated       = "stack.user.password.updated"
	QueueUserCreated               = "stack.user.created"
	QueueUserPhoneUpdated          = "stack.user.phone.updated"
	QueueDomainSubscriptionChanged = "stack.domain.subscription.changed"
	QueueUser2FAUpdated            = "stack.user.2fa.updated"
	QueueUserRecoveryEmailUpdated  = "stack.user.recovery-email.updated"
	QueueAppCommands               = "stack.app.commands.queue"
)

const (
	RoutingKeyUserPasswordUpdated         = "user.password.updated"
	RoutingKeyUserDeletionRequested       = "user.deletion.requested"
	RoutingKeyNextcloudMigrationRequested = "nextcloud.migration.requested"
)

// UserDeletionRequestedMessage is published when a user asks Twake to delete the account linked to the current cozy instance.
type UserDeletionRequestedMessage struct {
	WorkplaceFqdn string `json:"workplaceFqdn"`
	Reason        string `json:"reason"`
	RequestedBy   string `json:"requestedBy"`
	RequestedAt   int64  `json:"requestedAt"`
}

// NextcloudMigrationRequestedMessage is published when a user starts a
// Nextcloud to Cozy migration from the Settings UI. The external migration
// service consumes it, fetches an app audience token from the Cloudery, and
// orchestrates the transfer through the Stack's Nextcloud routes.
//
// Credentials for the Nextcloud account are stored in the io.cozy.accounts
// document referenced by AccountID. They MUST NOT be included in this
// message: the broker is not a trust boundary for secrets.
type NextcloudMigrationRequestedMessage struct {
	MigrationID   string `json:"migrationId"`
	WorkplaceFqdn string `json:"workplaceFqdn"`
	AccountID     string `json:"accountId"`
	SourcePath    string `json:"sourcePath,omitempty"`
	Timestamp     int64  `json:"timestamp"`
}
