// Package notes is about the documents of cozy-notes. The notes are persisted
// as files, but they also have some specific routes for enabling collaborative
// edition.
package notes

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/cozy/cozy-stack/model/note"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/files"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// CreateNote is the API handler for POST /notes. It creates a note, aka a file
// with a set of metadata to enable collaborative edition.
func CreateNote(c echo.Context) error {
	if err := middlewares.AllowWholeType(c, permission.POST, consts.Files); err != nil {
		return err
	}

	doc := &note.Document{}
	if _, err := jsonapi.Bind(c.Request().Body, doc); err != nil {
		return err
	}

	inst := middlewares.GetInstance(c)
	file, err := doc.Create(inst)
	if err != nil {
		return wrapError(err)
	}

	return files.FileData(c, http.StatusCreated, file, false, nil)
}

// GetNote is the API handler for GET /notes/:id. It fetches the file with the
// given id, and also includes the changes in the content that have been
// accepted by the stack but not yet persisted on the file.
func GetNote(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	fileID := c.Param("id")
	file, err := inst.VFS().FileByID(fileID)
	if err != nil {
		return wrapError(err)
	}

	if err := middlewares.AllowVFS(c, permission.GET, file); err != nil {
		return err
	}

	// TODO fetch the updated title and content from redis

	return files.FileData(c, http.StatusOK, file, false, nil)
}

// PatchNote is the API handler for PATCH /notes/:id. It applies some steps on
// the note document.
func PatchNote(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	fileID := c.Param("id")
	file, err := inst.VFS().FileByID(fileID)
	if err != nil {
		return wrapError(err)
	}

	if err := middlewares.AllowVFS(c, permission.PATCH, file); err != nil {
		return err
	}

	objs, err := jsonapi.BindCompound(c.Request().Body)
	if err != nil {
		return err
	}
	steps := make([]couchdb.JSONDoc, len(objs))
	for i, obj := range objs {
		if obj.Attributes == nil {
			return jsonapi.BadJSON()
		}
		if err = json.Unmarshal(*obj.Attributes, &steps[i]); err != nil {
			return wrapError(err)
		}
		steps[i].Type = consts.NotesSteps
	}

	ifMatch := c.Request().Header.Get("If-Match")
	if err := note.ApplySteps(inst, file, ifMatch, steps); err != nil {
		return wrapError(err)
	}

	return files.FileData(c, http.StatusOK, file, false, nil)
}

// ChangeTitle is the API handler for PUT /notes/:id/title. It updates the
// title and renames the file.
func ChangeTitle(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	fileID := c.Param("id")
	file, err := inst.VFS().FileByID(fileID)
	if err != nil {
		return wrapError(err)
	}

	if err := middlewares.AllowVFS(c, permission.PUT, file); err != nil {
		return err
	}

	doc := &note.Document{}
	if _, err := jsonapi.Bind(c.Request().Body, doc); err != nil {
		return err
	}

	if err := note.UpdateTitle(inst, file, doc.Title); err != nil {
		return wrapError(err)
	}

	return files.FileData(c, http.StatusOK, file, false, nil)
}

// Routes sets the routing for the collaborative edition of notes.
func Routes(router *echo.Group) {
	router.POST("", CreateNote)
	router.GET("/:id", GetNote)
	router.PATCH("/:id", PatchNote)
	router.PUT("/:id/title", ChangeTitle)
}

func wrapError(err error) *jsonapi.Error {
	switch err {
	case note.ErrInvalidSchema:
		return jsonapi.InvalidAttribute("schema", err)
	case note.ErrInvalidFile:
		return jsonapi.NotFound(err)
	case note.ErrNoSteps, note.ErrInvalidSteps:
		return jsonapi.BadRequest(err)
	case note.ErrCannotApply:
		return jsonapi.Conflict(err)
	case os.ErrNotExist, vfs.ErrParentDoesNotExist, vfs.ErrParentInTrash:
		return jsonapi.NotFound(err)
	case vfs.ErrFileTooBig:
		return jsonapi.Errorf(http.StatusRequestEntityTooLarge, "%s", err)
	}
	return jsonapi.InternalServerError(err)
}
