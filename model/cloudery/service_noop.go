package cloudery

import "github.com/cozy/cozy-stack/model/instance"

// NoopService implements [Service].
//
// This implem does nothing. It is used when no config is provided.
type NoopService struct{}

// SaveInstance does nothing.
func (s *NoopService) SaveInstance(inst *instance.Instance, cmd *SaveCmd) error {
	return nil
}

func (s *NoopService) HasBlockingSubscription(inst *instance.Instance) (bool, error) {
	return false, nil
}
