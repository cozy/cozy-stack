// Package note is the glue between the prosemirror models, the VFS, redis, the
// hub for realtime, etc.
package note

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/prosemirror-go/model"
)

// Document is the note document in memory. It is persisted to the VFS as a
// file, but with a debounce: the intermediate states are saved in Redis.
type Document struct {
	DocID    string          `json:"_id"`
	DocRev   string          `json:"_rev,omitempty"`
	Title    string          `json:"title"`
	DirID    string          `json:"dir_id,omitempty"`
	Revision int             `json:"revision"`
	Schema   json.RawMessage `json:"schema"`
	Content  json.RawMessage `json:"content"`
}

// ID returns the directory qualified identifier
func (d *Document) ID() string { return d.DocID }

// Rev returns the directory revision
func (d *Document) Rev() string { return d.DocRev }

// DocType returns the document type
func (d *Document) DocType() string { return consts.NotesDocuments }

// Clone implements couchdb.Doc
func (d *Document) Clone() couchdb.Doc {
	cloned := *d
	// XXX The schema is supposed to be immutable and, as such, is not cloned.
	return &cloned
}

// SetID changes the directory qualified identifier
func (d *Document) SetID(id string) { d.DocID = id }

// SetRev changes the directory revision
func (d *Document) SetRev(rev string) { d.DocRev = rev }

// Create the file in the VFS for this note.
func (d *Document) Create(inst *instance.Instance) (*vfs.FileDoc, error) {
	lock := inst.NotesLock()
	if err := lock.Lock(); err != nil {
		return nil, err
	}
	defer lock.Unlock()

	d.Revision = 0
	content, err := d.getInitialContent(inst)
	if err != nil {
		return nil, err
	}

	fileDoc, err := d.newFileDoc(inst, content)
	if err := writeFile(inst.VFS(), fileDoc, content); err != nil {
		return nil, err
	}
	return fileDoc, nil
}

func (d *Document) getInitialContent(inst *instance.Instance) ([]byte, error) {
	var spec model.SchemaSpec
	if err := json.Unmarshal(d.Schema, &spec); err != nil {
		inst.Logger().WithField("nspace", "notes").
			Infof("Cannot read the schema: %s", err)
		return nil, ErrInvalidSchema
	}

	schema, err := model.NewSchema(&spec)
	if err != nil {
		inst.Logger().WithField("nspace", "notes").
			Infof("Cannot instanciate the schema: %s", err)
		return nil, ErrInvalidSchema
	}

	node, err := schema.Node(schema.Spec.TopNode)
	if err != nil {
		inst.Logger().WithField("nspace", "notes").
			Infof("The topNode cannot be created: %s", err)
		return nil, ErrInvalidSchema
	}
	return []byte(node.String()), nil // TODO markdown
}

func (d *Document) getDirID() (string, error) {
	if d.DirID != "" {
		return d.DirID, nil
	}
	// TODO add a fallback on a default directory
	return "", errors.New("Not yet implemented")
}

func (d *Document) filename() string {
	if d.Title == "" {
		d.Title = "New note"
	}
	name := strings.ReplaceAll(d.Title, "/", "-")
	return name + ".cozy-note"
}

func (d *Document) newFileDoc(inst *instance.Instance, content []byte) (*vfs.FileDoc, error) {
	dirID, err := d.getDirID()
	if err != nil {
		return nil, err
	}

	fileDoc, err := vfs.NewFileDoc(
		d.filename(),
		dirID,
		int64(len(content)),
		nil, // Let the VFS compute the md5sum
		"text/markdown",
		"text",
		time.Now(),
		false, // Not executable
		false, // Not trashed
		nil,   // No tags
	)
	if err != nil {
		return nil, err
	}

	fileDoc.Metadata = d.metadata()
	fileDoc.CozyMetadata = vfs.NewCozyMetadata(inst.PageURL("/", nil))
	return fileDoc, nil
}

func (d *Document) metadata() map[string]interface{} {
	return map[string]interface{}{
		"content":  d.Content,
		"revision": d.Revision,
		"schema":   d.Schema,
	}
}

// TODO retry if another file with the same name already exists
func writeFile(fs vfs.VFS, fileDoc *vfs.FileDoc, content []byte) (err error) {
	var file vfs.File
	file, err = fs.CreateFile(fileDoc, nil)
	if err != nil {
		return
	}
	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	_, err = file.Write(content)
	return
}

var _ couchdb.Doc = &Document{}
