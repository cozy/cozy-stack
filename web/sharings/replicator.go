package sharings

import (
	"errors"
	"net/http"

	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
)

// RevsDiff is part of the replicator
func RevsDiff(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		inst.Logger().WithField("nspace", "replicator").Infof("Sharing was not found: %s", err)
		return wrapErrors(err)
	}
	var changed sharing.Changed
	if err = c.Bind(&changed); err != nil {
		inst.Logger().WithField("nspace", "replicator").Infof("Changes cannot be bound: %s", err)
		return wrapErrors(err)
	}
	if changed == nil {
		inst.Logger().WithField("nspace", "replicator").Infof("No changes")
		return echo.NewHTTPError(http.StatusBadRequest)
	}
	missings, err := s.ComputeRevsDiff(inst, changed)
	if err != nil {
		inst.Logger().WithField("nspace", "replicator").Infof("Error on compute: %s", err)
		return wrapErrors(err)
	}
	return c.JSON(http.StatusOK, missings)
}

// BulkDocs is part of the replicator
func BulkDocs(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		inst.Logger().WithField("nspace", "replicator").Infof("Sharing was not found: %s", err)
		return wrapErrors(err)
	}
	var docs sharing.DocsByDoctype
	if err = c.Bind(&docs); err != nil {
		inst.Logger().WithField("nspace", "replicator").Infof("Docs cannot be bound: %s", err)
		return wrapErrors(err)
	}
	if docs == nil {
		inst.Logger().WithField("nspace", "replicator").Infof("No bulk docs")
		return echo.NewHTTPError(http.StatusBadRequest)
	}
	err = s.ApplyBulkDocs(inst, docs)
	if err != nil {
		inst.Logger().WithField("nspace", "replicator").Infof("Error on apply: %s", err)
		return wrapErrors(err)
	}
	return c.JSON(http.StatusOK, []interface{}{})
}

// GetFolder returns informations about a folder
func GetFolder(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		inst.Logger().WithField("nspace", "replicator").Infof("Sharing was not found: %s", err)
		return wrapErrors(err)
	}
	member, err := requestMember(c, s)
	if err != nil {
		inst.Logger().WithField("nspace", "replicator").Infof("Member was not found: %s", err)
		return wrapErrors(err)
	}
	folder, err := s.GetFolder(inst, member, c.Param("id"))
	if err != nil {
		inst.Logger().WithField("nspace", "replicator").Infof("Folder was not found: %s", err)
		return wrapErrors(err)
	}
	return c.JSON(http.StatusOK, folder)
}

// SyncFile will try to synchronize a file from just its metadata. If it's not
// possible, it will respond with a key that allow to send the content to
// finish the synchronization.
func SyncFile(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		inst.Logger().WithField("nspace", "replicator").Infof("Sharing was not found: %s", err)
		return wrapErrors(err)
	}
	var fileDoc *sharing.FileDocWithRevisions
	if err = c.Bind(&fileDoc); err != nil {
		inst.Logger().WithField("nspace", "replicator").Infof("File cannot be bound: %s", err)
		return wrapErrors(err)
	}
	if c.Param("id") != fileDoc.DocID {
		err = errors.New("The identifiers in the URL and in the doc are not the same")
		return jsonapi.InvalidAttribute("id", err)
	}
	key, err := s.SyncFile(inst, fileDoc)
	if err != nil {
		inst.Logger().WithField("nspace", "replicator").Infof("Error on sync file: %s", err)
		return wrapErrors(err)
	}
	if key == nil {
		return c.NoContent(http.StatusNoContent)
	}
	return c.JSON(http.StatusOK, key)
}

// FileHandler is used to receive a file upload
func FileHandler(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		inst.Logger().WithField("nspace", "replicator").Infof("Sharing was not found: %s", err)
		return wrapErrors(err)
	}
	if err := s.HandleFileUpload(inst, c.Param("id"), c.Request().Body); err != nil {
		inst.Logger().WithField("nspace", "replicator").Infof("Error on file upload: %s", err)
		return wrapErrors(err)
	}
	return c.NoContent(http.StatusNoContent)
}

// EndInitial is used for ending the initial sync phase of a sharing
func EndInitial(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	sharingID := c.Param("sharing-id")
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		return wrapErrors(err)
	}
	if err := s.EndInitial(inst); err != nil {
		return wrapErrors(err)
	}
	return c.NoContent(http.StatusNoContent)
}

// replicatorRoutes sets the routing for the replicator
func replicatorRoutes(router *echo.Group) {
	group := router.Group("", checkSharingPermissions)
	group.POST("/:sharing-id/_revs_diff", RevsDiff, checkSharingWritePermissions)
	group.POST("/:sharing-id/_bulk_docs", BulkDocs, checkSharingWritePermissions)
	group.GET("/:sharing-id/io.cozy.files/:id", GetFolder, checkSharingReadPermissions)
	group.PUT("/:sharing-id/io.cozy.files/:id/metadata", SyncFile, checkSharingWritePermissions)
	group.PUT("/:sharing-id/io.cozy.files/:id", FileHandler, checkSharingWritePermissions)
	group.DELETE("/:sharing-id/initial", EndInitial, checkSharingWritePermissions)
}

func checkSharingReadPermissions(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		sharingID := c.Param("sharing-id")
		requestPerm, err := middlewares.GetPermission(c)
		if err != nil {
			middlewares.GetInstance(c).Logger().WithField("nspace", "replicator").
				Infof("Invalid permission: %s", err)
			return err
		}
		if !requestPerm.Permissions.AllowID("GET", consts.Sharings, sharingID) {
			middlewares.GetInstance(c).Logger().WithField("nspace", "replicator").
				Infof("Not allowed (%s)", sharingID)
			return echo.NewHTTPError(http.StatusForbidden)
		}
		return next(c)
	}
}

func checkSharingWritePermissions(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if err := hasSharingWritePermissions(c); err != nil {
			return err
		}
		return next(c)
	}
}

func hasSharingWritePermissions(c echo.Context) error {
	sharingID := c.Param("sharing-id")
	requestPerm, err := middlewares.GetPermission(c)
	if err != nil {
		middlewares.GetInstance(c).Logger().WithField("nspace", "replicator").
			Infof("Invalid permission: %s", err)
		return err
	}
	if !requestPerm.Permissions.AllowID("POST", consts.Sharings, sharingID) {
		middlewares.GetInstance(c).Logger().WithField("nspace", "replicator").
			Infof("Not allowed (%s)", sharingID)
		return echo.NewHTTPError(http.StatusForbidden)
	}
	return nil
}

func checkSharingPermissions(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		sharingID := c.Param("sharing-id")
		requestPerm, err := middlewares.GetPermission(c)
		if err != nil {
			middlewares.GetInstance(c).Logger().WithField("nspace", "replicator").
				Infof("Invalid permission: %s", err)
			return err
		}
		if !requestPerm.Permissions.AllowID("GET", consts.Sharings, sharingID) {
			middlewares.GetInstance(c).Logger().WithField("nspace", "replicator").
				Infof("Not allowed (%s)", sharingID)
			return echo.NewHTTPError(http.StatusForbidden)
		}
		return next(c)
	}
}

func requestMember(c echo.Context, s *sharing.Sharing) (*sharing.Member, error) {
	requestPerm, err := middlewares.GetPermission(c)
	if err != nil {
		return nil, err
	}
	return s.FindMemberByInboundClientID(requestPerm.SourceID)
}
