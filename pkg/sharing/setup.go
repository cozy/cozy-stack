package sharing

import (
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/instance"
)

// Setup is used when a member accept a sharing to prepare the io.cozy.shared
// database and start an initial replication. It is meant to be used in a new
// goroutine and, as such, does not return errors but log them.
func (s *Sharing) Setup(inst *instance.Instance, m *Member) {
	// Don't do the setup for most tests
	if !inst.OnboardingFinished {
		return
	}

	// TODO lock
	// TODO add triggers to update io.cozy.shared if not yet configured
	for i, rule := range s.Rules {
		if err := s.InitialCopy(inst, rule, i); err != nil {
			inst.Logger().Warnf("[sharing] Error on initial copy for %s: %s", rule.Title, err)
		}
	}
	// TODO add a trigger for next replications if not yet configured
	if err := s.ReplicateTo(inst, m, true); err != nil {
		inst.Logger().Warnf("[sharing] Error on initial replication: %s", err)
		s.retryReplicate(inst, 1)
	}
}

// InitialCopy lists the shared documents and put a reference in the
// io.cozy.shared database
func (s *Sharing) InitialCopy(inst *instance.Instance, rule Rule, r int) error {
	if rule.Local || len(rule.Values) == 0 {
		return nil
	}
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
