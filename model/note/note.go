// Package note is the glue between the prosemirror models, the VFS, redis, the
// hub for realtime, etc.
package note

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/prosemirror-go/model"
)

const (
	persistenceDebouce = "10m"
	cacheDuration      = 30 * time.Minute
	cleanStepsAfter    = 24 * time.Hour
)

// Document is the note document in memory. It is persisted to the VFS as a
// file, but with a debounce: the intermediate states are saved in Redis.
type Document struct {
	DocID      string                 `json:"_id"`
	DocRev     string                 `json:"_rev,omitempty"`
	DirID      string                 `json:"dir_id,omitempty"`
	Title      string                 `json:"title"`
	Version    int64                  `json:"version"`
	SchemaSpec map[string]interface{} `json:"schema"`
	RawContent map[string]interface{} `json:"content"`

	// Use cache for some computed properties
	schema   *model.Schema
	content  *model.Node
	markdown []byte
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
	d.markdown = nil
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
func (d *Document) Markdown() []byte {
	if len(d.markdown) == 0 {
		if content, err := d.Content(); err != nil {
			// TODO markdown
			d.markdown = []byte(content.String())
		}
	}
	return d.markdown
}

func (d *Document) getDirID(inst *instance.Instance) (string, error) {
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

func (d *Document) asFile(old *vfs.FileDoc) *vfs.FileDoc {
	file := old.Clone().(*vfs.FileDoc)
	file.Metadata = d.Metadata()
	file.ByteSize = int64(len(d.Markdown()))
	file.MD5Sum = nil // Let the VFS compute the md5sum
	if d.DirID != "" {
		file.DirID = d.DirID
	}

	// If the file was renamed manually before, we will keep its name. Else, we
	// can rename with the new title.
	oldTitle, _ := old.Metadata["title"].(string)
	if rename := titleToFilename(oldTitle) == old.DocName; rename {
		file.DocName = titleToFilename(d.Title)
		file.ResetFullpath()
	}

	file.UpdatedAt = time.Now()
	file.CozyMetadata.UpdatedAt = file.UpdatedAt
	return file
}

// Create the file in the VFS for this note.
func Create(inst *instance.Instance, doc *Document) (*vfs.FileDoc, error) {
	lock := inst.NotesLock()
	if err := lock.Lock(); err != nil {
		return nil, err
	}
	defer lock.Unlock()

	doc.Version = 0
	content, err := initialContent(inst, doc)
	if err != nil {
		return nil, err
	}
	doc.SetContent(content)

	file, err := writeFile(inst, doc, nil)
	if err != nil {
		return nil, err
	}
	if err := setupTrigger(inst, file.ID()); err != nil {
		return nil, err
	}
	return file, nil
}

func initialContent(inst *instance.Instance, doc *Document) (*model.Node, error) {
	schema, err := doc.Schema()
	if err != nil {
		inst.Logger().WithField("nspace", "notes").
			Infof("Cannot instantiate the schema: %s", err)
		return nil, ErrInvalidSchema
	}

	// Create an empty document that matches the schema constraints.
	typ, err := schema.NodeType(schema.Spec.TopNode)
	if err != nil {
		inst.Logger().WithField("nspace", "notes").
			Infof("The schema is invalid: %s", err)
		return nil, ErrInvalidSchema
	}
	node, err := typ.CreateAndFill()
	if err != nil {
		inst.Logger().WithField("nspace", "notes").
			Infof("The topNode cannot be created: %s", err)
		return nil, ErrInvalidSchema
	}
	return node, nil
}

func newFileDoc(inst *instance.Instance, doc *Document) (*vfs.FileDoc, error) {
	dirID, err := doc.getDirID(inst)
	if err != nil {
		return nil, err
	}
	content := doc.Markdown()

	fileDoc, err := vfs.NewFileDoc(
		titleToFilename(doc.Title),
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

	fileDoc.Metadata = doc.Metadata()
	fileDoc.CozyMetadata = vfs.NewCozyMetadata(inst.PageURL("/", nil))
	return fileDoc, nil
}

func titleToFilename(title string) string {
	if title == "" {
		title = "New note"
	}
	name := strings.ReplaceAll(title, "/", "-")
	return name + ".cozy-note"
}

func ensureNotesDir(inst *instance.Instance) (*vfs.DirDoc, error) {
	ref := couchdb.DocReference{
		Type: consts.Apps,
		ID:   consts.Apps + "/" + consts.NotesSlug,
	}
	key := []string{ref.Type, ref.ID}
	end := []string{ref.Type, ref.ID, couchdb.MaxString}
	req := &couchdb.ViewRequest{
		StartKey:    key,
		EndKey:      end,
		IncludeDocs: true,
	}
	var res couchdb.ViewResponse
	err := couchdb.ExecView(inst, couchdb.FilesReferencedByView, req, &res)
	if err != nil {
		return nil, err
	}

	fs := inst.VFS()
	if len(res.Rows) > 0 {
		return fs.DirByID(res.Rows[0].ID)
	}
	dirname := inst.Translate("Tree Notes")
	dir, err := vfs.NewDirDocWithPath(dirname, consts.RootDirID, "/", nil)
	if err != nil {
		return nil, err
	}
	dir.AddReferencedBy(ref)
	dir.CozyMetadata = vfs.NewCozyMetadata(inst.PageURL("/", nil))
	if err = fs.CreateDir(dir); err != nil {
		if !couchdb.IsConflictError(err) {
			return nil, err
		}
		dir, err = fs.DirByPath(dir.Fullpath)
		if err != nil {
			return nil, err
		}
		olddoc := dir.Clone().(*vfs.DirDoc)
		dir.AddReferencedBy(ref)
		_ = fs.UpdateDirDoc(olddoc, dir)
	}
	return dir, nil
}

// DebounceMessage is used by the trigger for saving the note to the VFS with a
// debounce.
type DebounceMessage struct {
	NoteID string `json:"note_id"`
}

func setupTrigger(inst *instance.Instance, fileID string) error {
	sched := job.System()
	msg := &DebounceMessage{NoteID: fileID}
	t, err := job.NewTrigger(inst, job.TriggerInfos{
		Type:       "@event",
		WorkerType: "notes-save",
		Arguments:  fmt.Sprintf("%s:UPDATED:%s", consts.NotesEvents, fileID),
		Debounce:   persistenceDebouce,
	}, msg)
	if err != nil {
		return err
	}
	return sched.AddTrigger(t)
}

func writeFile(inst *instance.Instance, doc *Document, oldDoc *vfs.FileDoc) (fileDoc *vfs.FileDoc, err error) {
	if oldDoc == nil {
		fileDoc, err = newFileDoc(inst, doc)
		if err != nil {
			return
		}
	} else {
		fileDoc = doc.asFile(oldDoc)
	}

	fs := inst.VFS()
	var file vfs.File
	file, err = fs.CreateFile(fileDoc, oldDoc)
	if err == os.ErrExist {
		filename := path.Base(fileDoc.DocName)
		suffix := time.Now().Format(time.RFC3339)
		fileDoc.DocName = fmt.Sprintf("%s - %s.cozy-note", filename, suffix)
		file, err = fs.CreateFile(fileDoc, oldDoc)
	}
	if err != nil {
		return
	}
	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = cerr
		}
		if err == nil {
			clearCache(inst, fileDoc.ID())
		}
	}()
	_, err = file.Write(doc.Markdown())
	return
}

// GetFile takes a file from the VFS as a note and returns its last version. It
// is useful when some changes have not yet been persisted to the VFS.
func GetFile(inst *instance.Instance, file *vfs.FileDoc) (*vfs.FileDoc, error) {
	lock := inst.NotesLock()
	if err := lock.Lock(); err != nil {
		return nil, err
	}
	defer lock.Unlock()
	doc, err := get(inst, file)
	if err != nil {
		return nil, err
	}
	return doc.asFile(file), nil
}

// get must be called with the notes lock already acquired. It will try to load
// the last version if a note from the cache, and if it fails, it will replay
// the new steps on the file from the VFS.
func get(inst *instance.Instance, file *vfs.FileDoc) (*Document, error) {
	if doc := getFromCache(inst, file.ID()); doc != nil {
		return doc, nil
	}
	// TODO replay steps
	return fromMetadata(file)
}

func fromMetadata(file *vfs.FileDoc) (*Document, error) {
	var version int64
	switch v := file.Metadata["version"].(type) {
	case float64:
		version = int64(v)
	case int64:
		version = v
	default:
		return nil, ErrInvalidFile
	}
	title, _ := file.Metadata["title"].(string)
	schema, ok := file.Metadata["schema"].(map[string]interface{})
	if !ok {
		return nil, ErrInvalidFile
	}
	content, ok := file.Metadata["content"].(map[string]interface{})
	if !ok {
		return nil, ErrInvalidFile
	}
	return &Document{
		DocID:      file.ID(),
		DirID:      file.DirID,
		Title:      title,
		Version:    version,
		SchemaSpec: schema,
		RawContent: content,
	}, nil
}

func getFromCache(inst *instance.Instance, noteID string) *Document {
	cache := config.GetConfig().CacheStorage
	buf, ok := cache.Get(cacheKey(inst, noteID))
	if !ok {
		return nil
	}
	var doc Document
	if err := json.Unmarshal(buf, &doc); err != nil {
		return nil
	}
	return &doc
}

func clearCache(inst *instance.Instance, noteID string) {
	cache := config.GetConfig().CacheStorage
	cache.Clear(cacheKey(inst, noteID))
}

func cacheKey(inst *instance.Instance, noteID string) string {
	return fmt.Sprintf("note:%s:%s", inst.Domain, noteID)
}

func saveToCache(inst *instance.Instance, doc *Document) error {
	cache := config.GetConfig().CacheStorage
	buf, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	cache.Set(cacheKey(inst, doc.ID()), buf, cacheDuration)
	return nil
}

// UpdateTitle changes the title of a note and renames the associated file.
func UpdateTitle(inst *instance.Instance, file *vfs.FileDoc, title string) (*vfs.FileDoc, error) {
	lock := inst.NotesLock()
	if err := lock.Lock(); err != nil {
		return nil, err
	}
	defer lock.Unlock()

	doc, err := get(inst, file)
	if err != nil {
		return nil, err
	}

	if doc.Title == title {
		return file, nil
	}
	doc.Title = title
	if err := saveToCache(inst, doc); err != nil {
		return nil, err
	}

	publishUpdatedTitle(inst, file.ID(), title)
	return doc.asFile(file), nil
}

func publishUpdatedTitle(inst *instance.Instance, fileID, title string) {
	event := Event{"title": title, "doctype": consts.NotesDocuments}
	event.SetID(fileID)
	event.publish(inst)
}

// Update is used to persist changes on a note to its file in the VFS.
func Update(inst *instance.Instance, fileID string) error {
	lock := inst.NotesLock()
	if err := lock.Lock(); err != nil {
		return err
	}
	defer lock.Unlock()

	old, err := inst.VFS().FileByID(fileID)
	if err != nil {
		return err
	}
	doc, err := get(inst, old)
	if err != nil {
		return err
	}

	if doc.Title == old.Metadata["title"] && doc.Version == old.Metadata["version"] {
		// Nothing to do
		return nil
	}

	_, err = writeFile(inst, doc, old)
	return err
}

var _ couchdb.Doc = &Document{}
