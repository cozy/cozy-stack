package bitwarden

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/cozy/cozy-stack/model/bitwarden"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/metadata"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

type folderRequest struct {
	Name string `json:"name"`
}

func (r *folderRequest) toFolder() *bitwarden.Folder {
	f := bitwarden.Folder{
		Name: r.Name,
	}
	md := metadata.New()
	md.DocTypeVersion = bitwarden.DocTypeVersion
	f.Metadata = md
	return &f
}

type folderResponse struct {
	ID     string    `json:"Id"`
	Name   string    `json:"Name"`
	Date   time.Time `json:"RevisionDate"`
	Object string    `json:"Object"`
}

func newFolderResponse(f *bitwarden.Folder) *folderResponse {
	r := folderResponse{
		ID:     f.CouchID,
		Name:   f.Name,
		Object: "folder",
	}
	if f.Metadata != nil {
		r.Date = f.Metadata.UpdatedAt.UTC()
	}
	return &r
}

// CreateFolder is the route to add a folder via the Bitwarden API.
func CreateFolder(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.POST, consts.BitwardenFolders); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid token",
		})
	}

	var req folderRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid JSON",
		})
	}
	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "missing name",
		})
	}

	folder := req.toFolder()
	if err := couchdb.CreateDoc(inst, folder); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}

	res := newFolderResponse(folder)
	return c.JSON(http.StatusOK, res)
}
