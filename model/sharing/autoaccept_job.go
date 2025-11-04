package sharing

import (
	"errors"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
)

// AutoAcceptMsg is the payload for auto-accepting a drive sharing from a trusted sender
type AutoAcceptMsg struct {
	SharingID string `json:"sharing_id"`
	State     string `json:"state"`
}

// EnqueueAutoAccept schedules a job to auto-accept a drive sharing
func EnqueueAutoAccept(inst *instance.Instance, sharingID, state string) error {
	if inst == nil || sharingID == "" || state == "" {
		return ErrInvalidSharing
	}

	msg, err := job.NewMessage(&AutoAcceptMsg{
		SharingID: sharingID,
		State:     state,
	})
	if err != nil {
		return err
	}

	_, err = job.System().PushJob(inst, &job.JobRequest{
		WorkerType: "share-autoaccept",
		Message:    msg,
	})
	return err
}

// HandleAutoAccept executes the auto-acceptance for a Drive sharing.
// The OAuth state must be provided by the owner in the sharing request.
func HandleAutoAccept(inst *instance.Instance, msg *AutoAcceptMsg) error {
	if inst == nil || msg == nil || msg.SharingID == "" || msg.State == "" {
		return ErrInvalidSharing
	}

	s, err := FindSharing(inst, msg.SharingID)
	if err != nil {
		return err
	}

	// Send the acceptance answer using the state provided by the owner
	if err := s.SendAnswer(inst, msg.State); err != nil {
		if errors.Is(err, ErrAlreadyAccepted) {
			return nil // Already accepted, not an error
		}
		return err
	}
	return nil
}
