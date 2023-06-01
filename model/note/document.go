package note

import (
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
	DocID      string                 `json:"_id"`
	DocRev     string                 `json:"_rev,omitempty"`
	CreatedBy  string                 `json:"-"`
	DirID      string                 `json:"dir_id,omitempty"` // Only used at creation
	Title      string                 `json:"title"`
	Version    int64                  `json:"version"`
	SchemaSpec map[string]interface{} `json:"schema"`
	RawContent map[string]interface{} `json:"content"`

	// Use cache for some computed properties
	schema  *model.Schema
	content *model.Node
}

// ID returns the document qualified identifier
func (d *Document) ID() string { return d.DocID }

// Rev returns the document revision
func (d *Document) Rev() string { return d.DocRev }

// DocType returns the document type
func (d *Document) DocType() string { return consts.NotesDocuments }

// Clone implements couchdb.Doc
func (d *Document) Clone() couchdb.Doc {
	cloned := *d
	// XXX The schema and the content are supposed to be immutable and, as
	// such, are not cloned.
	return &cloned
}

// SetID changes the document qualified identifier
func (d *Document) SetID(id string) { d.DocID = id }

// SetRev changes the document revision
func (d *Document) SetRev(rev string) { d.DocRev = rev }

// Metadata returns the file metadata for this note.
func (d *Document) Metadata() map[string]interface{} {
	return map[string]interface{}{
		"title":   d.Title,
		"content": d.RawContent,
		"version": d.Version,
		"schema":  d.SchemaSpec,
	}
}

// Schema returns the prosemirror schema for this note
func (d *Document) Schema() (*model.Schema, error) {
	if d.schema == nil {
		spec := model.SchemaSpecFromJSON(d.SchemaSpec)
		schema, err := model.NewSchema(&spec)
		if err != nil {
			return nil, ErrInvalidSchema
		}
		d.schema = schema
	}
	return d.schema, nil
}

// SetContent updates the content of this note, and clears the cache.
func (d *Document) SetContent(content *model.Node) {
	d.RawContent = content.ToJSON()
	d.content = content
}

// Content returns the prosemirror content for this note.
func (d *Document) Content() (*model.Node, error) {
	if d.content == nil {
		if len(d.RawContent) == 0 {
			return nil, ErrInvalidFile
		}
		schema, err := d.Schema()
		if err != nil {
			return nil, err
		}
		content, err := model.NodeFromJSON(schema, d.RawContent)
		if err != nil {
			return nil, err
		}
		d.content = content
	}
	return d.content, nil
}

// Markdown returns a markdown serialization of the content.
func (d *Document) Markdown(images []*Image) ([]byte, error) {
	content, err := d.Content()
	if err != nil {
		return nil, err
	}
	md := markdownSerializer(images).Serialize(content)
	return []byte(md), nil
}

// GetDirID returns the ID of the directory where the note will be created.
func (d *Document) GetDirID(inst *instance.Instance) (string, error) {
	if d.DirID != "" {
		return d.DirID, nil
	}
	parent, err := ensureNotesDir(inst)
	if err != nil {
		return "", err
	}
	d.DirID = parent.ID()
	return d.DirID, nil
}

func (d *Document) asFile(inst *instance.Instance, old *vfs.FileDoc) *vfs.FileDoc {
	now := time.Now()
	file := old.Clone().(*vfs.FileDoc)
	file.Metadata = d.Metadata()
	file.Mime = consts.NoteMimeType
	file.MD5Sum = nil // Let the VFS compute the md5sum

	// If the file was renamed manually before, we will keep its name. Else, we
	// can rename with the new title.
	newTitle := titleToFilename(inst, d.Title, old.CreatedAt)
	oldTitle, _ := old.Metadata["title"].(string)
	rename := d.Title != "" && titleToFilename(inst, oldTitle, old.CreatedAt) == old.DocName
	if old.DocName == "" {
		rename = true
	}
	if strings.Contains(old.DocName, " - conflict - ") && oldTitle != newTitle {
		rename = true
	}
	if rename {
		file.DocName = newTitle
		file.ResetFullpath()
		_, _ = file.Path(inst.VFS()) // Prefill the fullpath
	}

	file.UpdatedAt = now
	file.CozyMetadata.UpdatedAt = file.UpdatedAt
	return file
}
