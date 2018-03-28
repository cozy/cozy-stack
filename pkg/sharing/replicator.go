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
	"github.com/cozy/cozy-stack/pkg/jobs"
	multierror "github.com/hashicorp/go-multierror"
)

// MaxRetries is the maximal number of retries for a replicator
const MaxRetries = 10

// BatchSize is the maximal number of documents mainpulated at once by the replicator
const BatchSize = 100

// ReplicateMsg is used for jobs on the share-replicate worker.
type ReplicateMsg struct {
	SharingID string `json:"sharing_id"`
	Errors    int    `json:"errors"`
}

// Replicate starts a replicator on this sharing.
func (s *Sharing) Replicate(inst *instance.Instance, errors int) error {
	// TODO lock
	var errm error
	if !s.Owner {
		errm = s.ReplicateTo(inst, &s.Members[0], false)
	} else {
		for i, m := range s.Members {
			if i == 0 {
				continue
			}
			if m.Status == MemberStatusReady {
				err := s.ReplicateTo(inst, &s.Members[i], false)
				errm = multierror.Append(errm, err)
			}
		}
	}
	if errm != nil {
		s.retryReplicate(inst, errors+1)
	}
	return errm
}

// retryReplicate will add a job to retry a failed replication
func (s *Sharing) retryReplicate(inst *instance.Instance, errors int) {
	if errors == MaxRetries {
		inst.Logger().Warnf("[sharing] Max retries reached")
		return
	}
	// TODO add a delay between retries
	msg, err := jobs.NewMessage(&ReplicateMsg{
		SharingID: s.SID,
		Errors:    errors,
	})
	if err != nil {
		inst.Logger().Warnf("[sharing] Error on retry to replicate: %s", err)
		return
	}
	_, err = jobs.System().PushJob(&jobs.JobRequest{
		Domain:     inst.Domain,
		WorkerType: "share-replicate",
		Message:    msg,
	})
	if err != nil {
		inst.Logger().Warnf("[sharing] Error on retry to replicate: %s", err)
	}
}

// ReplicateTo starts a replicator on this sharing to the given member.
// http://docs.couchdb.org/en/2.1.1/replication/protocol.html
// https://github.com/pouchdb/pouchdb/blob/master/packages/node_modules/pouchdb-replication/src/replicate.js
// TODO check for errors
// TODO pouch use the pending property of changes for its replicator
// https://github.com/pouchdb/pouchdb/blob/master/packages/node_modules/pouchdb-replication/src/replicate.js#L298-L301
func (s *Sharing) ReplicateTo(inst *instance.Instance, m *Member, initial bool) error {
	if m.Instance == "" {
		return ErrInvalidURL
	}

	lastSeq, err := s.getLastSeqNumber(inst, m)
	if err != nil {
		return err
	}
	fmt.Printf("lastSeq = %s\n", lastSeq)

	changes, seq, err := s.callChangesFeed(inst, lastSeq)
	if err != nil {
		return err
	}
	if seq == lastSeq {
		return nil
	}
	fmt.Printf("changes = %#v\n", changes)
	// TODO filter the changes according to the sharing rules

	if len(*changes) > 0 {
		var missings *Missings
		if initial {
			missings = transformChangesInMissings(changes)
		} else {
			missings, err = s.callRevsDiff(m, changes)
			if err != nil {
				return err
			}
		}
		fmt.Printf("missings = %#v\n", missings)

		docs, err := s.getMissingDocs(inst, missings)
		if err != nil {
			return err
		}
		fmt.Printf("docs = %#v\n", docs)

		err = s.sendBulkDocs(m, docs)
		if err != nil {
			return err
		}
	}

	return s.UpdateLastSequenceNumber(inst, m, seq)
}

// getLastSeqNumber returns the last sequence number of the previous
// replication to this member
func (s *Sharing) getLastSeqNumber(inst *instance.Instance, m *Member) (string, error) {
	id, err := s.replicationID(m)
	if err != nil {
		return "", err
	}
	result, err := couchdb.GetLocal(inst, consts.Shared, id)
	if couchdb.IsNotFoundError(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	seq, _ := result["last_seq"].(string)
	return seq, nil
}

// UpdateLastSequenceNumber updates the last sequence number for this
// replication if it's superior to the number in CouchDB
func (s *Sharing) UpdateLastSequenceNumber(inst *instance.Instance, m *Member, seq string) error {
	id, err := s.replicationID(m)
	if err != nil {
		return err
	}
	result, err := couchdb.GetLocal(inst, consts.Shared, id)
	if err != nil {
		if !couchdb.IsNotFoundError(err) {
			return err
		}
		result = make(map[string]interface{})
	} else {
		if prev, ok := result["last_seq"].(string); ok {
			if RevGeneration(seq) <= RevGeneration(prev) {
				return nil
			}
		}
	}
	result["last_seq"] = seq
	return couchdb.PutLocal(inst, consts.Shared, id, result)
}

// replicationID gives an identifier for this replicator
func (s *Sharing) replicationID(m *Member) (string, error) {
	for i := range s.Members {
		if &s.Members[i] == m {
			id := fmt.Sprintf("sharing-%s-%d", s.SID, i)
			return id, nil
		}
	}
	return "", ErrMemberNotFound
}

// Changes is a map of "doctype-docid" -> [revisions]
// It's the format for the request body of our _revs_diff
type Changes map[string][]string

// callChangesFeed fetches the last changes from the changes feed
// http://docs.couchdb.org/en/2.1.1/api/database/changes.html
// TODO add a filter on the sharing
// TODO what if there are more changes in the feed that BatchSize?
func (s *Sharing) callChangesFeed(inst *instance.Instance, since string) (*Changes, string, error) {
	response, err := couchdb.GetChanges(inst, &couchdb.ChangesRequest{
		DocType:     consts.Shared,
		IncludeDocs: true,
		Since:       since,
		Limit:       BatchSize,
	})
	if err != nil {
		return nil, "", err
	}
	changes := make(Changes)
	for _, r := range response.Results {
		changes[r.DocID] = make([]string, len(r.Changes))
		for i, c := range r.Changes {
			changes[r.DocID][i] = c.Rev
		}
	}
	return &changes, response.LastSeq, nil
}

// Missings is a struct for the response of _revs_diff
type Missings map[string]MissingEntry

// MissingEntry is a struct with the missing revisions for an id
type MissingEntry struct {
	Missing []string `json:"missing"`
	PAs     []string `json:"possible_ancestors"`
}

// transformChangesInMissings is used for the initial replication (revs_diff is
// not called), to prepare the payload for the bulk_get calls
func transformChangesInMissings(changes *Changes) *Missings {
	missings := make(Missings, len(*changes))
	for key, revs := range *changes {
		missings[key] = MissingEntry{
			Missing: []string{revs[len(revs)-1]},
		}
	}
	return &missings
}

// callRevsDiff asks the other cozy to compute the _revs_diff
// http://docs.couchdb.org/en/2.1.1/api/database/misc.html#db-revs-diff
func (s *Sharing) callRevsDiff(m *Member, changes *Changes) (*Missings, error) {
	u, err := url.Parse(m.Instance)
	if err != nil {
		return nil, err
	}
	// "doctype-docid" -> [leaf revisions]
	leafRevs := make(map[string][]string, len(*changes))
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
	seen := make(map[int]struct{})
	for _, rev := range wants {
		g := RevGeneration(rev)
		if _, ok := seen[g]; !ok {
			wgs = append(wgs, g)
		}
		seen[g] = struct{}{}
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
		for _, rev := range result.Revisions {
			if _, ok := result.Infos[s.SID]; !ok {
				continue
			}
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
// TODO what if we fetch an old revision on a compacted database?
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
	ids := make([]string, 0, BatchSize)
	for doctype, docs := range payload {
		for _, doc := range docs {
			id, ok := doc["_id"].(string)
			if !ok {
				return ErrMissingID
			}
			ids = append(ids, doctype+"/"+id)
		}
	}
	refs, err := FindReferences(inst, ids)
	if err != nil {
		return err
	}

	for doctype, docs := range payload {
		toAdd, toUpdate := partitionDocsWithRefs(docs, refs)

		if err := couchdb.BulkForceUpdateDocs(inst, doctype, append(toAdd, toUpdate...)); err != nil {
			return err
		}
		// TODO remove this assertion when this function will be stabilized
		if len(refs) < len(docs) {
			panic("too much refs have not been consumed")
		}
		refs = refs[len(docs):]
	}

	// TODO remove this assertion when this function will be stabilized
	if len(refs) != 0 {
		panic("all refs have not been consumed")
	}

	// TODO update the io.cozy.shared database
	// TODO call rtevent

	return nil
}

// partitionDocsWithRefs returns two slices: the first with documents that have
// no shared reference, the second with documents that have one
func partitionDocsWithRefs(docs []map[string]interface{}, refs []*SharedRef) ([]map[string]interface{}, []map[string]interface{}) {
	toAdd := make([]map[string]interface{}, 0, len(docs))
	toUpdate := make([]map[string]interface{}, 0, len(docs))
	for i, doc := range docs {
		if refs[i] == nil {
			toAdd = append(toAdd, doc)
		} else {
			toUpdate = append(toUpdate, doc)
		}
	}
	return toAdd, toUpdate
}
