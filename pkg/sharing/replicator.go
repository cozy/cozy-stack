package sharing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

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
// http://docs.couchdb.org/en/2.1.1/replication/protocol.html
// https://github.com/pouchdb/pouchdb/blob/master/packages/node_modules/pouchdb-replication/src/replicate.js
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
	// TODO pouch use the pending property of changes for its replicator
	// https://github.com/pouchdb/pouchdb/blob/master/packages/node_modules/pouchdb-replication/src/replicate.js#L298-L301

	missings, err := s.callRevsDiff(m, changes)
	if err != nil {
		return err
	}
	fmt.Printf("missings = %#v\n", missings)

	docs, err := s.getMissingDocs(inst, missings)
	if err != nil {
		return err
	}
	fmt.Printf("docs = %#v\n", docs)

	err = s.sendBulkDocs(m, docs)
	return err

	// TODO check for errors
	// TODO save the sequence number
}

// Changes is a map of "doctype-docid" -> [revisions]
// It's the format for the request body of our _revs_diff
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

// Missings is a struct for the response of _revs_diff
type Missings map[string]MissingEntry

// MissingEntry is a struct with the missing revisions for an id
type MissingEntry struct {
	Missing []string `json:"missing"`
	PAs     []string `json:"possible_ancestors"`
}

// callRevsDiff asks the other cozy to compute the _revs_diff
// http://docs.couchdb.org/en/2.1.1/api/database/misc.html#db-revs-diff
func (s *Sharing) callRevsDiff(m *Member, changes *Changes) (*Missings, error) {
	u, err := url.Parse(m.Instance)
	if err != nil {
		return nil, err
	}
	leafRevs := make(map[string][]string) // "doctype-docid" -> [leaf revisions]
	for key, revs := range *changes {
		leafRevs[key] = revs[len(revs)-1:]
	}
	body, err := json.Marshal(leafRevs)
	if err != nil {
		return nil, err
	}
	res, err := request.Req(&request.Options{
		Method: http.MethodPost,
		Scheme: u.Scheme,
		Domain: u.Host,
		Path:   "/sharings/" + s.SID + "/_revs_diff",
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

// computePossibleAncestors find the revisions in haves that have a generation
// number just inferior to a generation number of a revision in wants.
func computePossibleAncestors(wants []string, haves []string) []string {
	// Build a sorted array of unique generation number for revisions of wants
	var wgs []int
	seen := make(map[int]bool)
	for _, rev := range wants {
		g := RevGeneration(rev)
		if !seen[g] {
			wgs = append(wgs, g)
		}
		seen[g] = true
	}
	sort.Ints(wgs)

	var pas []string
	i := 0
	for j, rev := range haves {
		g := RevGeneration(rev)
		found := false
		for i < len(wgs) && g >= wgs[i] {
			found = true
			i++
		}
		if found && j > 0 {
			pas = append(pas, haves[j-1])
		}
	}
	if i != len(wgs) {
		pas = append(pas, haves[len(haves)-1])
	}

	return pas
}

// ComputeRevsDiff takes a map of id->[revisions] and returns the missing
// revisions for those documents on the current instance.
func (s *Sharing) ComputeRevsDiff(inst *instance.Instance, changes Changes) (*Missings, error) {
	ids := make([]string, 0, len(changes))
	for id := range changes {
		ids = append(ids, id)
	}
	results := make([]SharedRef, 0, len(changes))
	req := couchdb.AllDocsRequest{Keys: ids}
	err := couchdb.GetAllDocs(inst, consts.Shared, &req, &results)
	if err != nil {
		return nil, err
	}
	missings := make(Missings)
	for id, revs := range changes {
		missings[id] = MissingEntry{Missing: revs}
	}
	for _, result := range results {
		if _, ok := changes[result.SID]; !ok {
			continue
		}
		// TODO check that result.Info[s.SID] exists
		for _, rev := range result.Revisions {
			for i, r := range changes[result.SID] {
				if rev == r {
					change := changes[result.SID]
					changes[result.SID] = append(change[:i], change[i+1:]...)
					break
				}
			}
		}
		if len(changes[result.SID]) == 0 {
			delete(missings, result.SID)
		} else {
			pas := computePossibleAncestors(changes[result.SID], result.Revisions)
			missings[result.SID] = MissingEntry{
				Missing: changes[result.SID],
				PAs:     pas,
			}
		}
	}
	return &missings, nil
}

// DocsByDoctype is a map of doctype -> slice of documents of this doctype
type DocsByDoctype map[string][]map[string]interface{}

// getMissingDocs fetches the documents in bulk, partitionned by their doctype.
// https://github.com/apache/couchdb-documentation/pull/263/files
// TODO use the possible ancestors
func (s *Sharing) getMissingDocs(inst *instance.Instance, missings *Missings) (*DocsByDoctype, error) {
	queries := make(map[string][]couchdb.IDRev) // doctype -> payload for _bulk_get
	for key, missing := range *missings {
		parts := strings.SplitN(key, "/", 2)
		if len(parts) != 2 {
			return nil, ErrInternalServerError
		}
		doctype := parts[0]
		for _, rev := range missing.Missing {
			ir := couchdb.IDRev{ID: parts[1], Rev: rev}
			queries[doctype] = append(queries[doctype], ir)
		}
	}

	var docs DocsByDoctype
	for doctype, query := range queries {
		results, err := couchdb.BulkGetDocs(inst, doctype, query)
		if err != nil {
			return nil, err
		}
		docs[doctype] = append(docs[doctype], results...)
	}
	return &docs, nil
}

// sendBulkDocs takes a bulk of documents and send them to the other cozy
// http://docs.couchdb.org/en/2.1.1/api/database/bulk-api.html#db-bulk-docs
// https://wiki.apache.org/couchdb/HTTP_Bulk_Document_API#Posting_Existing_Revisions
// https://gist.github.com/nono/42aee18de6314a621f9126f284e303bb
func (s *Sharing) sendBulkDocs(m *Member, docs *DocsByDoctype) error {
	u, err := url.Parse(m.Instance)
	if err != nil {
		return err
	}
	body, err := json.Marshal(docs)
	if err != nil {
		return err
	}
	res, err := request.Req(&request.Options{
		Method: http.MethodPost,
		Scheme: u.Scheme,
		Domain: u.Host,
		Path:   "/sharings/" + s.SID + "/_bulk_docs",
		Headers: request.Headers{
			"Accept":       "application/json",
			"Content-Type": "application/json",
		},
		Body: bytes.NewReader(body),
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode/100 == 5 {
		return ErrInternalServerError
	}
	if res.StatusCode/100 != 2 {
		return ErrClientError
	}
	return nil
}

// ApplyBulkDocs is a multi-doctypes version of the POST _bulk_docs endpoint of CouchDB
func (s *Sharing) ApplyBulkDocs(inst *instance.Instance, payload DocsByDoctype) error {
	for doctype, docs := range payload {
		// TODO what if the database for doctype does not exist
		// TODO update the io.cozy.shared database
		// TODO call rtevent
		if err := couchdb.BulkForceUpdateDocs(inst, doctype, docs); err != nil {
			return err
		}
	}
	return nil
}
