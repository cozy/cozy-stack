package sharing

import (
	"context"
	"errors"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/messaging"
	"github.com/cozy/cozy-stack/pkg/rabbitmq"
)

// CollectedContactsWorker is the worker that publishes collected contacts.
const CollectedContactsWorker = "share-collected-contacts"

var errNoCollectedPublisher = errors.New("sharing: no publisher for the collected contacts")

// CollectedContactsMsg is the job message: the emails of the new contacts.
type CollectedContactsMsg struct {
	Emails []string `json:"emails"`
}

// EnqueueCollectedContacts pushes the publish job. Errors are only logged.
func EnqueueCollectedContacts(inst *instance.Instance, emails []string) {
	if !collectedContactsEnabled(inst, emails) {
		return
	}
	msg, err := job.NewMessage(&CollectedContactsMsg{Emails: emails})
	if err == nil {
		_, err = job.System().PushJob(inst, &job.JobRequest{
			WorkerType: CollectedContactsWorker,
			Message:    msg,
		})
	}
	if err != nil {
		inst.Logger().WithNamespace("sharing").
			Warnf("Cannot schedule the publication of %d collected contacts: %s", len(emails), err)
	}
}

// PublishCollectedContacts sends the new contacts to Sabre in one message.
func PublishCollectedContacts(ctx context.Context, inst *instance.Instance, publisher messaging.Publisher, emails []string) error {
	if !collectedContactsEnabled(inst, emails) {
		return nil
	}
	if publisher == nil {
		return errNoCollectedPublisher
	}
	return publisher.Publish(ctx, collectedRequest(inst, emails))
}

// collectedContactsEnabled tells if there is something to publish.
func collectedContactsEnabled(inst *instance.Instance, emails []string) bool {
	return len(emails) > 0 && inst.HasCommonContacts() && inst.Email != ""
}

func collectedRequest(inst *instance.Instance, emails []string) messaging.PublishRequest {
	cards := make([]map[string]interface{}, 0, len(emails))
	for _, email := range emails {
		cards = append(cards, map[string]interface{}{
			"@type":  "Card",
			"emails": map[string]interface{}{"e1": map[string]interface{}{"address": email}},
		})
	}
	return messaging.PublishRequest{
		ContextName: inst.ContextName,
		Exchange:    rabbitmq.ExchangeContactsCollected,
		Fanout:      true,
		Payload: rabbitmq.CollectedContactsMessage{
			UserEmail:         inst.Email,
			CollectedContacts: cards,
		},
	}
}
