package sharing

import (
	"fmt"
	"runtime"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/lock"
)

// SetupReceiver is used on the receivers' cozy to make sure the cozy can
// receive the shared documents.
func (s *Sharing) SetupReceiver(inst *instance.Instance) error {
	if err := couchdb.EnsureDBExist(inst, consts.Shared); err != nil {
		return err
	}
	if s.HasFiles() {
		return EnsureSharedWithMeDir(inst)
	}
	if err := s.AddTrackTriggers(inst); err != nil {
		return err
	}
	// TODO do not add the share-replicate trigger for a one-way sharing
	if err := s.AddReplicateTrigger(inst); err != nil {
		return err
	}
	return nil
}

// Setup is used when a member accepts a sharing to prepare the io.cozy.shared
// database and start an initial replication. It is meant to be used in a new
// goroutine and, as such, does not return errors but log them.
func (s *Sharing) Setup(inst *instance.Instance, m *Member) {
	defer func() {
		if r := recover(); r != nil {
			var err error
			switch r := r.(type) {
			case error:
				err = r
			default:
				err = fmt.Errorf("%v", r)
			}
			stack := make([]byte, 4<<10) // 4 KB
			length := runtime.Stack(stack, false)
			log := inst.Logger().WithField("panic", true)
			log.Errorf("PANIC RECOVER %s: %s", err.Error(), stack[:length])
		}
	}()

	// Don't do the setup for most tests
	if !inst.OnboardingFinished {
		return
	}

	mu := lock.ReadWrite(inst.Domain + "/sharings/" + s.SID)
	mu.Lock()
	defer mu.Unlock()

	if err := couchdb.EnsureDBExist(inst, consts.Shared); err != nil {
		inst.Logger().Warnf("[sharing] Can't ensure io.cozy.shared exists (%s): %s", s.SID, err)
	}

	if err := s.AddTrackTriggers(inst); err != nil {
		inst.Logger().Warnf("[sharing] Errors on setup of track triggers (%s): %s", s.SID, err)
	}
	// TODO add triggers for rules that can revoke the sharing
	for i, rule := range s.Rules {
		if err := s.InitialCopy(inst, rule, i); err != nil {
			inst.Logger().Warnf("[sharing] Error on initial copy for %s (%s): %s", rule.Title, s.SID, err)
		}
	}
	if err := s.AddReplicateTrigger(inst); err != nil {
		inst.Logger().Warnf("[sharing] Error on setup replicate trigger (%s): %s", s.SID, err)
	}
	if err := s.ReplicateTo(inst, m, true); err != nil {
		inst.Logger().Warnf("[sharing] Error on initial replication (%s): %s", s.SID, err)
		s.retryReplicate(inst, 1)
	}
}

// AddTrackTriggers creates the share-track triggers for each rule of the
// sharing that will update the io.cozy.shared database.
func (s *Sharing) AddTrackTriggers(inst *instance.Instance) error {
	if s.Triggers.Track {
		return nil
	}
	sched := jobs.System()
	for i, rule := range s.Rules {
		args := rule.TriggerArgs(s.Owner)
		if args == "" {
			continue
		}
		msg, err := jobs.NewMessage(&TrackMessage{
			SharingID: s.SID,
			RuleIndex: i,
			DocType:   rule.DocType,
		})
		if err != nil {
			return err
		}
		t, err := jobs.NewTrigger(&jobs.TriggerInfos{
			Domain:     inst.Domain,
			Type:       "@event",
			WorkerType: "share-track",
			Message:    msg,
			Arguments:  args,
		})
		if err != nil {
			return err
		}
		if err = sched.AddTrigger(t); err != nil {
			return err
		}
	}
	s.Triggers.Track = true
	return couchdb.UpdateDoc(inst, s)
}

// AddReplicateTrigger creates the share-replicate trigger for this sharing:
// it will starts the replicator when some changes are made to the
// io.cozy.shared database.
func (s *Sharing) AddReplicateTrigger(inst *instance.Instance) error {
	if s.Triggers.Replicate {
		return nil
	}
	msg, err := jobs.NewMessage(&ReplicateMsg{
		SharingID: s.SID,
		Errors:    0,
	})
	if err != nil {
		return err
	}
	args := consts.Shared + ":CREATED,UPDATED:" + s.SID + ":sharing"
	t, err := jobs.NewTrigger(&jobs.TriggerInfos{
		Domain:     inst.Domain,
		Type:       "@event",
		WorkerType: "share-replicate",
		Message:    msg,
		Arguments:  args,
		Debounce:   "5s",
	})
	if err != nil {
		return err
	}
	sched := jobs.System()
	if err = sched.AddTrigger(t); err != nil {
		return err
	}
	s.Triggers.Replicate = true
	return couchdb.UpdateDoc(inst, s)
}

// InitialCopy lists the shared documents and put a reference in the
// io.cozy.shared database
func (s *Sharing) InitialCopy(inst *instance.Instance, rule Rule, r int) error {
	if rule.Local || len(rule.Values) == 0 {
		return nil
	}

	mu := lock.ReadWrite(inst.Domain + "/shared")
	mu.Lock()
	defer mu.Unlock()

	docs, err := findDocsToCopy(inst, rule)
	if err != nil {
		return err
	}
	refs, err := s.buildReferences(inst, rule, r, docs)
	if err != nil {
		return err
	}
	refs = compactSlice(refs)
	if len(refs) == 0 {
		return nil
	}
	return couchdb.BulkUpdateDocs(inst, consts.Shared, refs)
}

// findDocsToCopy finds the documents that match the given rule
func findDocsToCopy(inst *instance.Instance, rule Rule) ([]couchdb.JSONDoc, error) {
	var docs []couchdb.JSONDoc
	if rule.Selector == "" || rule.Selector == "id" {
		req := &couchdb.AllDocsRequest{
			Keys: rule.Values,
		}
		if err := couchdb.GetAllDocs(inst, rule.DocType, req, &docs); err != nil {
			return nil, err
		}
	} else {
		// Create index based on selector to retrieve documents to share
		name := "by-" + rule.Selector
		idx := mango.IndexOnFields(rule.DocType, name, []string{rule.Selector})
		if err := couchdb.DefineIndex(inst, idx); err != nil {
			return nil, err
		}
		// Request the index for all values
		for _, val := range rule.Values {
			var results []couchdb.JSONDoc
			req := &couchdb.FindRequest{
				UseIndex: name,
				Selector: mango.Equal(rule.Selector, val),
			}
			if err := couchdb.FindDocs(inst, rule.DocType, req, &results); err != nil {
				return nil, err
			}
			docs = append(docs, results...)
		}
	}
	return docs, nil
}

// buildReferences build the SharedRef to add/update the given docs in the
// io.cozy.shared database
func (s *Sharing) buildReferences(inst *instance.Instance, rule Rule, r int, docs []couchdb.JSONDoc) ([]interface{}, error) {
	ids := make([]string, len(docs))
	for i, doc := range docs {
		ids[i] = rule.DocType + "/" + doc.ID()
	}
	srefs, err := FindReferences(inst, ids)
	if err != nil {
		return nil, err
	}

	refs := make([]interface{}, len(docs))
	for i, doc := range docs {
		rev := doc.Rev()
		if srefs[i] == nil {
			refs[i] = SharedRef{
				SID:       rule.DocType + "/" + doc.ID(),
				Revisions: []string{rev},
				Infos: map[string]SharedInfo{
					s.SID: {Rule: r},
				},
			}
		} else {
			found := false
			for _, revision := range srefs[i].Revisions {
				if revision == rev {
					found = true
					break
				}
			}
			if found {
				if _, ok := srefs[i].Infos[s.SID]; ok {
					continue
				}
			} else {
				srefs[i].Revisions = append(srefs[i].Revisions, rev)
			}
			srefs[i].Infos[s.SID] = SharedInfo{Rule: r}
			refs[i] = *srefs[i]
		}
	}

	return refs, nil
}

// compactSlice returns the given slice without the nil values
// https://github.com/golang/go/wiki/SliceTricks#filtering-without-allocating
func compactSlice(a []interface{}) []interface{} {
	b := a[:0]
	for _, x := range a {
		if x != nil {
			b = append(b, x)
		}
	}
	return b
}
