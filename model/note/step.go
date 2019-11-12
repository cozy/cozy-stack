package note

import (
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/prosemirror-go/model"
	"github.com/cozy/prosemirror-go/transform"
)

// Step is a patch to apply on a note.
type Step map[string]interface{}

// ID returns the directory qualified identifier
func (s Step) ID() string {
	id, _ := s["_id"].(string)
	return id
}

// Rev returns the directory revision
func (s Step) Rev() string {
	rev, _ := s["_rev"].(string)
	return rev
}

// DocType returns the document type
func (s Step) DocType() string { return consts.NotesSteps }

// Clone implements couchdb.Doc
func (s Step) Clone() couchdb.Doc {
	cloned := make(Step)
	for k, v := range s {
		cloned[k] = v
	}
	return cloned
}

// SetID changes the step qualified identifier
func (s Step) SetID(id string) { s["_id"] = id }

// SetRev changes the step revision
func (s Step) SetRev(rev string) { s["_rev"] = rev }

// Included is part of the jsonapi.Object interface
func (s Step) Included() []jsonapi.Object { return nil }

// Links is part of the jsonapi.Object interface
func (s Step) Links() *jsonapi.LinksList { return nil }

// Relationships is part of the jsonapi.Object interface
func (s Step) Relationships() jsonapi.RelationshipMap { return nil }

// ApplySteps takes a note and some steps, and tries to apply them. It is an
// all or nothing change: if there is one error, the note won't be changed.
// TODO fetch last info for file (if debounce)
func ApplySteps(inst *instance.Instance, file *vfs.FileDoc, lastRev string, steps []Step) error {
	lock := inst.NotesLock()
	if err := lock.Lock(); err != nil {
		return err
	}
	defer lock.Unlock()

	if len(steps) == 0 {
		return ErrNoSteps
	}

	oldContent, ok := file.Metadata["content"].(map[string]interface{})
	if !ok {
		return ErrInvalidFile
	}
	revision, ok := file.Metadata["revision"].(string)
	if !ok {
		return ErrInvalidFile
	}
	if lastRev != revision {
		return ErrCannotApply
	}

	schemaSpec, ok := file.Metadata["schema"].(map[string]interface{})
	if !ok {
		return ErrInvalidSchema
	}

	spec := model.SchemaSpecFromJSON(schemaSpec)
	schema, err := model.NewSchema(&spec)
	if err != nil {
		inst.Logger().WithField("nspace", "notes").
			Infof("Cannot instantiate the schema: %s", err)
		return ErrInvalidSchema
	}

	doc, err := model.NodeFromJSON(schema, oldContent)
	if err != nil {
		inst.Logger().WithField("nspace", "notes").
			Infof("Cannot instantiate the document: %s", err)
		return ErrInvalidFile
	}

	for _, s := range steps {
		step, err := transform.StepFromJSON(schema, s)
		if err != nil {
			inst.Logger().WithField("nspace", "notes").
				Infof("Cannot instantiate a step: %s", err)
			return ErrInvalidSteps
		}
		result := step.Apply(doc)
		if result.Failed != "" {
			inst.Logger().WithField("nspace", "notes").
				Infof("Cannot apply a step: %s (revision=%s)", result.Failed, revision)
			return ErrCannotApply
		}
		doc = result.Doc
		revision += "1" // TODO
	}

	olds := make([]interface{}, len(steps))
	docs := make([]interface{}, len(steps))
	for i, s := range steps {
		docs[i] = s
	}
	if err := couchdb.BulkUpdateDocs(inst, consts.NotesSteps, docs, olds); err != nil {
		if !couchdb.IsNoDatabaseError(err) {
			return err
		}
		if err := couchdb.EnsureDBExist(inst, consts.NotesSteps); err != nil {
			return err
		}
		if err := couchdb.BulkUpdateDocs(inst, consts.NotesSteps, docs, olds); err != nil {
			return err
		}
	}
	// TODO purge the old steps

	olddoc := file.Clone().(*vfs.FileDoc)
	file.Metadata["content"] = doc.ToJSON()
	file.Metadata["revision"] = revision
	// TODO markdown
	markdown := []byte(doc.String())

	// TODO add debounce
	file.ByteSize = int64(len(markdown))
	file.MD5Sum = nil
	return writeFile(inst.VFS(), file, olddoc, markdown)
}

// GetSteps returns the steps for the given note, starting from the revision.
func GetSteps(inst *instance.Instance, file *vfs.FileDoc, rev string) ([]Step, error) {
	var steps []Step
	req := couchdb.AllDocsRequest{
		Limit: 1000,
		// TODO StartKey: file.DocID + "/" + rev,
	}
	if err := couchdb.GetAllDocs(inst, consts.NotesSteps, &req, &steps); err != nil {
		return nil, err
	}
	// TODO
	if len(steps) == 0 || len(steps) == 1000 {
		return nil, ErrTooOld
	}
	return steps, nil
}

var _ jsonapi.Object = &Step{}
