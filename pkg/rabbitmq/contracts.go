package rabbitmq

const (
	ExchangeAuth     = "auth"
	ExchangeRAGIndex = "rag.index.topic"
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
	RoutingKeyUserPasswordUpdated   = "user.password.updated"
	RoutingKeyUserDeletionRequested = "user.deletion.requested"
	RoutingKeyRAGIndexFile          = "rag.index.file"
	RoutingKeyRAGIndexDelete        = "rag.index.delete"
)

// UserDeletionRequestedMessage is published when a user asks Twake to delete the account linked to the current cozy instance.
type UserDeletionRequestedMessage struct {
	WorkplaceFqdn string `json:"workplaceFqdn"`
	Reason        string `json:"reason"`
	RequestedBy   string `json:"requestedBy"`
	RequestedAt   int64  `json:"requestedAt"`
}
