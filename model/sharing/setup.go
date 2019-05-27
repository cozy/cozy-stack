package sharing

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/lock"
)

// SetupReceiver is used on the receivers' cozy to make sure the cozy can
// receive the shared documents.
func (s *Sharing) SetupReceiver(inst *instance.Instance) error {
	inst.Logger().WithField("nspace", "sharing").
		Debugf("Setup receiver on %#v", inst)

	if err := couchdb.EnsureDBExist(inst, consts.Shared); err != nil {
		return err
	}
	if err := s.AddTrackTriggers(inst); err != nil {
		return err
	}
	if !s.ReadOnly() {
		if err := s.AddReplicateTrigger(inst); err != nil {
			return err
		}
		if s.FirstFilesRule() != nil {
			if err := s.AddUploadTrigger(inst); err != nil {
				return err
			}
			// The sharing directory is created when the stack receives the
			// first file (it allows to not create it if it's just a file that
			// is shared, not a directory or an album). But, for an empty
			// directory, no files are sent, so we need to create it now.
			if s.NbFiles == 0 {
				if _, err := s.GetSharingDir(inst); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Setup is used when a member accepts a sharing to prepare the io.cozy.shared
// database and start an initial replication. It is meant to be used in a new
// goroutine and, as such, does not return errors but log them.
func (s *Sharing) Setup(inst *instance.Instance, m *Member) {
	// Don't do the setup for most tests
	if !inst.OnboardingFinished {
		return
	}
	inst.Logger().WithField("nspace", "sharing").
		Debugf("Setup for member %#v on %#v", m, inst)

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
			log := inst.Logger().WithField("panic", true).WithField("nspace", "sharing")
			log.Errorf("PANIC RECOVER %s: %s", err.Error(), stack[:length])
		}
	}()

	mu := lock.ReadWrite(inst, "sharings/"+s.SID)
	if err := mu.Lock(); err != nil {
		return
	}
	defer mu.Unlock()

	if err := couchdb.EnsureDBExist(inst, consts.Shared); err != nil {
		inst.Logger().WithField("nspace", "sharing").
			Warnf("Can't ensure io.cozy.shared exists (%s): %s", s.SID, err)
	}
	if rule := s.FirstFilesRule(); rule != nil && rule.Selector != couchdb.SelectorReferencedBy {
		if err := s.AddReferenceForSharingDir(inst, rule); err != nil {
			inst.Logger().WithField("nspace", "sharing").
				Warnf("Error on referenced_by for the sharing dir (%s): %s", s.SID, err)
		}
	}

	if err := s.AddTrackTriggers(inst); err != nil {
		inst.Logger().WithField("nspace", "sharing").
			Warnf("Error on setup of track triggers (%s): %s", s.SID, err)
	}
	for i, rule := range s.Rules {
		if err := s.InitialCopy(inst, rule, i); err != nil {
			inst.Logger().Warnf("Error on initial copy for %s (%s): %s", rule.Title, s.SID, err)
		}
	}
	if err := s.AddReplicateTrigger(inst); err != nil {
		inst.Logger().WithField("nspace", "sharing").
			Warnf("Error on setup replicate trigger (%s): %s", s.SID, err)
	}
	if pending, err := s.ReplicateTo(inst, m, true); err != nil {
		inst.Logger().WithField("nspace", "sharing").
			Warnf("Error on initial replication (%s): %s", s.SID, err)
		s.retryWorker(inst, "share-replicate", 0)
	} else {
		if pending {
			s.pushJob(inst, "share-replicate")
		}
		if s.FirstFilesRule() == nil {
			return
		}
		if err := s.AddUploadTrigger(inst); err != nil {
			inst.Logger().WithField("nspace", "sharing").
				Warnf("Error on setup upload trigger (%s): %s", s.SID, err)
		}
		if err := s.InitialUpload(inst, m); err != nil {
			inst.Logger().WithField("nspace", "sharing").
				Warnf("Error on initial upload (%s): %s", s.SID, err)
			s.retryWorker(inst, "share-upload", 0)
		}
	}

	go s.NotifyRecipients(inst, m)
}

// AddTrackTriggers creates the share-track triggers for each rule of the
// sharing that will update the io.cozy.shared database.
func (s *Sharing) AddTrackTriggers(inst *instance.Instance) error {
	if s.Triggers.TrackID != "" {
		return nil
	}
	sched := job.System()
	for i, rule := range s.Rules {
		args := rule.TriggerArgs()
		if args == "" {
			continue
		}
		msg := &TrackMessage{
			SharingID: s.SID,
			RuleIndex: i,
			DocType:   rule.DocType,
		}
		t, err := job.NewTrigger(inst, job.TriggerInfos{
			Type:       "@event",
			WorkerType: "share-track",
			Arguments:  args,
		}, msg)
		if err != nil {
			return err
		}
		if err = sched.AddTrigger(t); err != nil {
			return err
		}
		s.Triggers.TrackID = t.ID()
	}
	return couchdb.UpdateDoc(inst, s)
}

// AddReplicateTrigger creates the share-replicate trigger for this sharing:
// it will starts the replicator when some changes are made to the
// io.cozy.shared database.
func (s *Sharing) AddReplicateTrigger(inst *instance.Instance) error {
	if s.Triggers.ReplicateID != "" {
		return nil
	}
	msg := &ReplicateMsg{
		SharingID: s.SID,
		Errors:    0,
	}
	args := consts.Shared + ":CREATED,UPDATED:" + s.SID + ":sharing"
	t, err := job.NewTrigger(inst, job.TriggerInfos{
		Domain:     inst.ContextualDomain(),
		Type:       "@event",
		WorkerType: "share-replicate",
		Arguments:  args,
		Debounce:   "5s",
	}, msg)
	inst.Logger().WithField("nspace", "sharing").Infof("Create trigger %#v", t)
	if err != nil {
		return err
	}
	sched := job.System()
	if err = sched.AddTrigger(t); err != nil {
		return err
	}
	s.Triggers.ReplicateID = t.ID()
	return couchdb.UpdateDoc(inst, s)
}

// InitialCopy lists the shared documents and put a reference in the
// io.cozy.shared database
func (s *Sharing) InitialCopy(inst *instance.Instance, rule Rule, r int) error {
	if rule.Local || len(rule.Values) == 0 {
		return nil
	}

	mu := lock.ReadWrite(inst, "shared")
	if err := mu.Lock(); err != nil {
		return err
	}
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
	olds := make([]interface{}, len(refs))
	return couchdb.BulkUpdateDocs(inst, consts.Shared, refs, olds)
}

// findDocsToCopy finds the documents that match the given rule
func findDocsToCopy(inst *instance.Instance, rule Rule) ([]couchdb.JSONDoc, error) {
	var docs []couchdb.JSONDoc
	if rule.Selector == "" || rule.Selector == "id" {
		if rule.DocType == consts.Files {
			for _, fileID := range rule.Values {
				err := vfs.WalkByID(inst.VFS(), fileID, func(name string, dir *vfs.DirDoc, file *vfs.FileDoc, err error) error {
					if err != nil {
						return err
					}
					if dir != nil {
						if dir.DocID != fileID {
							docs = append(docs, dirToJSONDoc(dir))
						}
					} else if file != nil {
						docs = append(docs, fileToJSONDoc(file))
					}
					return nil
				})
				if err != nil {
					return nil, err
				}
			}
		} else {
			req := &couchdb.AllDocsRequest{
				Keys: rule.Values,
			}
			if err := couchdb.GetAllDocs(inst, rule.DocType, req, &docs); err != nil {
				return nil, err
			}
		}
	} else {
		if rule.Selector == couchdb.SelectorReferencedBy {
			for _, val := range rule.Values {
				req := &couchdb.ViewRequest{
					Key:         strings.SplitN(val, "/", 2),
					IncludeDocs: true,
					Reduce:      false,
				}
				var res couchdb.ViewResponse
				err := couchdb.ExecView(inst, couchdb.FilesReferencedByView, req, &res)
				if err != nil {
					return nil, err
				}
				for _, row := range res.Rows {
					var doc couchdb.JSONDoc
					if err = json.Unmarshal(row.Doc, &doc); err == nil {
						docs = append(docs, doc)
					}
				}
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
		info := SharedInfo{
			Rule:   r,
			Binary: rule.DocType == consts.Files && doc.Get("type") == consts.FileType,
		}
		if srefs[i] == nil {
			refs[i] = SharedRef{
				SID:       rule.DocType + "/" + doc.ID(),
				Revisions: &RevsTree{Rev: rev},
				Infos:     map[string]SharedInfo{s.SID: info},
			}
		} else {
			found := srefs[i].Revisions.Find(rev) != nil
			if found {
				if _, ok := srefs[i].Infos[s.SID]; ok {
					continue
				}
			} else {
				srefs[i].Revisions.Add(rev)
			}
			srefs[i].Infos[s.SID] = info
			refs[i] = *srefs[i]
		}
	}

	return refs, nil
}

// AddUploadTrigger creates the share-upload trigger for this sharing:
// it will starts the synchronization of the binaries when a file is added or
// updated in the io.cozy.shared database.
func (s *Sharing) AddUploadTrigger(inst *instance.Instance) error {
	if s.Triggers.UploadID != "" {
		return nil
	}
	msg := &UploadMsg{
		SharingID: s.SID,
		Errors:    0,
	}
	args := consts.Shared + ":CREATED,UPDATED:" + s.SID + ":sharing"
	t, err := job.NewTrigger(inst, job.TriggerInfos{
		Domain:     inst.ContextualDomain(),
		Type:       "@event",
		WorkerType: "share-upload",
		Arguments:  args,
		Debounce:   "5s",
	}, msg)
	inst.Logger().WithField("nspace", "sharing").Infof("Create trigger %#v", t)
	if err != nil {
		return err
	}
	sched := job.System()
	if err = sched.AddTrigger(t); err != nil {
		return err
	}
	s.Triggers.UploadID = t.ID()
	return couchdb.UpdateDoc(inst, s)
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
