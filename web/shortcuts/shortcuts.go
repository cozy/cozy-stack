// Package shortcuts is about the .url shortcuts. They are files and, as such,
// can be manipulated via the /files API. But the stack also offer some routes
// to make it easier to create and open them.
package shortcuts

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/shortcut"
	"github.com/cozy/cozy-stack/web/files"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// Shortcut is a struct with the high-level information about a .url file.
type Shortcut struct {
	DocID    string       `json:"_id"`
	DocRev   string       `json:"_rev,omitempty"`
	Name     string       `json:"name"`
	DirID    string       `json:"dir_id"`
	URL      string       `json:"url"`
	Metadata vfs.Metadata `json:"metadata"`
}

// ID returns the shortcut qualified identifier
func (s *Shortcut) ID() string { return s.DocID }

// Rev returns the shortcut revision
func (s *Shortcut) Rev() string { return s.DocRev }

// DocType returns the shortcut type
func (s *Shortcut) DocType() string { return consts.FilesShortcuts }

// Clone implements couchdb.Doc
func (s *Shortcut) Clone() couchdb.Doc {
	cloned := *s
	cloned.Metadata = make(vfs.Metadata, len(s.Metadata))
	for k, v := range s.Metadata {
		cloned.Metadata[k] = v
	}
	return &cloned
}

// SetID changes the shortcut qualified identifier
func (s *Shortcut) SetID(id string) { s.DocID = id }

// SetRev changes the shortcut revision
func (s *Shortcut) SetRev(rev string) { s.DocRev = rev }

// Relationships is a method of the jsonapi.Document interface
func (s *Shortcut) Relationships() jsonapi.RelationshipMap { return nil }

// Included is a method of the jsonapi.Document interface
func (s *Shortcut) Included() []jsonapi.Object { return nil }

// Links is a method of the jsonapi.Document interface
func (s *Shortcut) Links() *jsonapi.LinksList { return nil }

// Create is the API handler for POST /shortcuts. It can be used to create a
// shortcut from a JSON description.
func Create(c echo.Context) error {
	if err := middlewares.AllowWholeType(c, permission.POST, consts.Files); err != nil {
		return err
	}

	doc := &Shortcut{}
	if _, err := jsonapi.Bind(c.Request().Body, doc); err != nil {
		return err
	}
	if doc.URL == "" {
		return jsonapi.InvalidAttribute("url", errors.New("No URL"))
	}
	if doc.Name == "" {
		return jsonapi.InvalidAttribute("name", errors.New("No name"))
	}
	if !strings.HasSuffix(doc.Name, ".url") {
		doc.Name = doc.Name + ".url"
	}
	if doc.DirID == "" {
		doc.DirID = consts.RootDirID
	}

	body := shortcut.Generate(doc.URL)
	cm, _ := files.CozyMetadataFromClaims(c, true)
	fileDoc, err := vfs.NewFileDoc(
		doc.Name,
		doc.DirID,
		int64(len(body)),
		nil, // Let the VFS compute the md5sum
		consts.ShortcutMimeType,
		"shortcut",
		cm.UpdatedAt,
		false, // Not executable
		false, // Not trashed
		nil,   // No tags
	)
	if err != nil {
		return wrapError(err)
	}
	fileDoc.Metadata = doc.Metadata
	fileDoc.CozyMetadata = cm

	inst := middlewares.GetInstance(c)
	file, err := inst.VFS().CreateFile(fileDoc, nil)
	if err != nil {
		return wrapError(err)
	}
	defer func() {
	}()
	_, err = file.Write(body)
	if cerr := file.Close(); cerr != nil && err == nil {
		err = cerr
	}
	if err != nil {
		return wrapError(err)
	}

	return files.FileData(c, http.StatusCreated, fileDoc, false, nil)
}

// Get is the API handler for GET /shortcuts/:id. It follows the link or send a
// JSON-API response with information about the shortcut, depending on the
// Accept header.
func Get(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	fs := inst.VFS()
	fileID := c.Param("id")
	file, err := fs.FileByID(fileID)
	if err != nil {
		return wrapError(err)
	}

	if err := middlewares.AllowVFS(c, permission.GET, file); err != nil {
		return err
	}

	f, err := fs.OpenFile(file)
	if err != nil {
		return wrapError(err)
	}
	defer f.Close()
	link, err := shortcut.Parse(f)
	if err != nil {
		return wrapError(err)
	}
	if link.URL == "" {
		return jsonapi.BadRequest(errors.New("No URL found"))
	}

	accept := c.Request().Header.Get(echo.HeaderAccept)
	if strings.Contains(accept, echo.MIMEApplicationJSON) ||
		strings.Contains(accept, jsonapi.ContentType) {
		doc := &Shortcut{
			DocID:    file.DocID,
			DocRev:   file.DocRev,
			Name:     file.DocName,
			DirID:    file.DirID,
			URL:      link.URL,
			Metadata: file.Metadata,
		}
		return jsonapi.Data(c, http.StatusOK, doc, nil)
	}

	return c.Redirect(http.StatusSeeOther, link.URL)
}

// Routes set the routing for the shortcuts.
func Routes(router *echo.Group) {
	router.POST("", Create)
	router.GET("/:id", Get)
}

func wrapError(err error) *jsonapi.Error {
	switch err {
	case os.ErrNotExist, vfs.ErrParentDoesNotExist, vfs.ErrParentInTrash:
		return jsonapi.NotFound(err)
	case vfs.ErrFileTooBig:
		return jsonapi.Errorf(http.StatusRequestEntityTooLarge, "%s", err)
	case shortcut.ErrInvalidShortcut:
		return jsonapi.BadRequest(err)
	}
	return jsonapi.InternalServerError(err)
}
