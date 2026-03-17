package rabbitmq

const (
	ExchangeAuth = "auth"
)

const (
	QueueUserPasswordUpdated       = "stack.user.password.updated"
	QueueUserCreated               = "stack.user.created"
	QueueUserPhoneUpdated          = "stack.user.phone.updated"
	QueueDomainSubscriptionChanged = "stack.domain.subscription.changed"
	QueueAppCommands               = "stack.app.commands.queue"
)

const (
	RoutingKeyUserPasswordUpdated   = "user.password.updated"
	RoutingKeyUserDeletionRequested = "user.deletion.requested"
)

// UserDeletionRequestedMessage is published when a user asks Twake to delete the account linked to the current cozy instance.
type UserDeletionRequestedMessage struct {
	Email       string `json:"email"`
	Reason      string `json:"reason"`
	RequestedBy string `json:"requestedBy"`
	RequestedAt int64  `json:"requestedAt"`
}
