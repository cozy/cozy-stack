package sharing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/client/request"
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

	changes, err := s.callChangesFeed(inst)
	if err != nil {
		return err
	}
	fmt.Printf("changes = %#v\n", changes)

	missings, err := s.callRevsDiff(m, changes)
	if err != nil {
		return err
	}
	fmt.Printf("missings = %#v\n", missings)

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

// Changes is a map of "doctype-docid" -> [revisions]
// It's the format for the request body of our revs_diff
type Changes map[string][]string

// callChangesFeed fetches the last changes from the changes feed
// http://docs.couchdb.org/en/2.1.1/api/database/changes.html
// TODO add Limit, add Since, add a filter on the sharing
func (s *Sharing) callChangesFeed(inst *instance.Instance) (*Changes, error) {
	response, err := couchdb.GetChanges(inst, &couchdb.ChangesRequest{
		DocType:     consts.Shared,
		IncludeDocs: true,
	})
	if err != nil {
		return nil, err
	}
	changes := make(Changes)
	for _, r := range response.Results {
		changes[r.DocID] = make([]string, len(r.Changes))
		for i, c := range r.Changes {
			changes[r.DocID][i] = c.Rev
		}
	}
	return &changes, nil
}

// Missings is a struct for the response of revs_diff
type Missings map[string]struct {
	Missing   []string `json:"missing"`
	Ancestors []string `json:"possible_ancestors,omitempty"`
}

// callRevsDiff asks the other cozy to compute the revs_diff
// http://docs.couchdb.org/en/2.1.1/api/database/misc.html#db-revs-diff
func (s *Sharing) callRevsDiff(m *Member, changes *Changes) (*Missings, error) {
	u, err := url.Parse(m.Instance)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(changes)
	if err != nil {
		return nil, err
	}
	res, err := request.Req(&request.Options{
		Method: http.MethodPost,
		Scheme: u.Scheme,
		Domain: u.Host,
		Path:   "/sharings/" + s.SID + "/revs_diff",
		Headers: request.Headers{
			"Accept":       "application/json",
			"Content-Type": "application/json",
		},
		Body: bytes.NewReader(body),
	})
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode/100 == 5 {
		return nil, ErrInternalServerError
	}
	if res.StatusCode/100 != 2 {
		return nil, ErrClientError
	}
	missings := make(Missings)
	if err = json.NewDecoder(res.Body).Decode(&missings); err != nil {
		return nil, err
	}
	return &missings, nil
}
