package sharing

import (
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/logger"
)

// Setup is used when a member accept a sharing to prepare the io.cozy.shared
// database and start an initial replication. It is meant to be used in a new
// goroutine and, as such, does not return errors but log them.
func (s *Sharing) Setup(inst *instance.Instance, m *Member) {
	// TODO lock
	// TODO add triggers to update io.cozy.shared if not yet configured
	// TODO copy to io.cozy.shared
	// TODO add a trigger for next replications if not yet configured
	if err := s.ReplicateTo(inst, m, true); err != nil {
		logger.WithDomain(inst.Domain).Warnf("[sharing] Error on initial replication: %s", err)
		s.retryReplicate(inst, 1)
	}
}
