package sharing

import (
	"fmt"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	multierror "github.com/hashicorp/go-multierror"
)

// ReplicateMsg is used for jobs on the share-replicate worker.
type ReplicateMsg struct {
	SharingID string `json:"sharing_id"`
}

// Replicate starts a replicator on this sharing.
func (s *Sharing) Replicate(inst *instance.Instance) error {
	if !s.Owner {
		return s.ReplicateTo(inst, &s.Members[0])
	}
	var errm error
	for i, m := range s.Members {
		if i == 0 {
			continue
		}
		if m.Status == MemberStatusReady {
			err := s.ReplicateTo(inst, &s.Members[i])
			errm = multierror.Append(errm, err)
		}
	}
	return errm
}

// ReplicateTo starts a replicator on this sharing to the given member.
// See http://docs.couchdb.org/en/2.1.1/replication/protocol.html
func (s *Sharing) ReplicateTo(inst *instance.Instance, m *Member) error {
	if m.Instance == "" {
		return ErrInvalidURL
	}

	// TODO get the last sequence number

	// Get the last changes from the changes feed
	// http://docs.couchdb.org/en/2.1.1/api/database/changes.html
	// TODO add a limit, add Since, add a filter on the sharing
	changes, err := couchdb.GetChanges(inst, &couchdb.ChangesRequest{
		DocType:     consts.Shared,
		IncludeDocs: true,
	})
	if err != nil {
		return err
	}
	fmt.Printf("changes = %#v\n", changes)

	// Ask the other cozy to compute the revs_diff
	// http://docs.couchdb.org/en/2.1.1/api/database/misc.html#db-revs-diff
	// TODO missings := callRevsDiff(changes)

	// Regroup the missing revisions by doctypes
	// TODO byDoctypes := partitionByDoctype(missings)

	// for doctype, ids := range byDoctypes {
	// Get the documents in a bulk
	// http://docs.couchdb.org/en/2.1.1/api/database/bulk-api.html#post--db-_all_docs
	// TODO docs := getBulkDocs(ids)

	// Send them in a bulk
	// http://docs.couchdb.org/en/2.1.1/api/database/bulk-api.html#db-bulk-docs
	// TODO responses := sendBulkDocs(docs)
	// TODO check for errors
	// }

	// TODO save the sequence number

	return nil
}
