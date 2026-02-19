package sharings

import (
	"errors"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/files"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/notes"
	"github.com/cozy/cozy-stack/web/office"
	webperm "github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo/v4"
)

type docPatch struct {
	docID string

	vfs.DocPatch
}

// ListSharedDrives returns the list of the shared drives.
func ListSharedDrives(c echo.Context) error {
	if err := middlewares.AllowWholeType(c, permission.GET, consts.Files); err != nil {
		return wrapErrors(err)
	}

	inst := middlewares.GetInstance(c)
	drives, err := sharing.ListDrives(inst)
	if err != nil {
		return wrapErrors(err)
	}

	objs := make([]jsonapi.Object, 0, len(drives))
	for _, drive := range drives {
		obj := &sharing.APISharing{
			Sharing:     drive,
			Credentials: nil,
		}
		objs = append(objs, obj)
	}
	return jsonapi.DataList(c, http.StatusOK, objs, nil)
}

// CreateSharedDrive creates a new shared drive from an existing folder.
// POST /sharings/drives
func CreateSharedDrive(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	var attrs struct {
		Description string `json:"description"`
		FolderID    string `json:"folder_id"`
	}
	obj, err := jsonapi.Bind(c.Request().Body, &attrs)
	if err != nil {
		return jsonapi.BadJSON()
	}

	if attrs.FolderID == "" {
		return jsonapi.InvalidParameter("folder_id", errors.New("folder_id is required"))
	}

	// Create the sharing from folder first (builds the rules)
	newSharing, err := sharing.CreateDrive(inst, attrs.FolderID, attrs.Description, "")
	if err != nil {
		return wrapErrors(err)
	}

	// Check permissions using the existing function (validates against the rules)
	slug, err := checkCreatePermissions(c, newSharing)
	if err != nil {
		return echo.NewHTTPError(http.StatusForbidden)
	}

	// Set the app slug if obtained from permissions
	if slug != "" && newSharing.AppSlug == "" {
		newSharing.AppSlug = slug
	}

	// Extract recipient IDs from relationships
	rwGroupIDs, rwContactIDs := extractRecipientIDs(obj, "recipients")
	roGroupIDs, roContactIDs := extractRecipientIDs(obj, "read_only_recipients")

	// Create the sharing document first (drives can be created without recipients)
	if _, err = newSharing.Create(inst); err != nil {
		return wrapErrors(err)
	}

	// Add read-write recipients and send invitations
	if len(rwGroupIDs) > 0 || len(rwContactIDs) > 0 {
		if err = newSharing.AddGroupsAndContacts(inst, rwGroupIDs, rwContactIDs, false); err != nil {
			return wrapErrors(err)
		}
	}

	// Add read-only recipients and send invitations
	if len(roGroupIDs) > 0 || len(roContactIDs) > 0 {
		if err = newSharing.AddGroupsAndContacts(inst, roGroupIDs, roContactIDs, true); err != nil {
			return wrapErrors(err)
		}
	}

	as := &sharing.APISharing{
		Sharing:     newSharing,
		Credentials: nil,
		SharedDocs:  nil,
	}
	return jsonapi.Data(c, http.StatusCreated, as, nil)
}

// extractRecipientIDs extracts group and contact IDs from a JSON:API relationship.
func extractRecipientIDs(obj *jsonapi.ObjectMarshalling, relationshipName string) (groupIDs, contactIDs []string) {
	rel, ok := obj.GetRelationship(relationshipName)
	if !ok {
		return nil, nil
	}
	data, ok := rel.Data.([]interface{})
	if !ok {
		return nil, nil
	}
	for _, ref := range data {
		refMap, ok := ref.(map[string]interface{})
		if !ok {
			continue
		}
		id, ok := refMap["id"].(string)
		if !ok {
			continue
		}
		if t, _ := refMap["type"].(string); t == consts.Groups {
			groupIDs = append(groupIDs, id)
		} else {
			contactIDs = append(contactIDs, id)
		}
	}
	return groupIDs, contactIDs
}

// Load either a DirDoc or a FileDoc from the given `file-id` param. The
// function also checks permissions.
func loadDirOrFileFromParam(c echo.Context, inst *instance.Instance, perm permission.Verb) (*vfs.DirDoc, *vfs.FileDoc, error) {
	dir, file, err := inst.VFS().DirOrFileByID(c.Param("file-id"))
	if err != nil {
		return nil, nil, files.WrapVfsError(err)
	}

	if dir != nil {
		err = middlewares.AllowVFS(c, perm, dir)
	} else {
		err = middlewares.AllowVFS(c, perm, file)
	}
	if err != nil {
		return nil, nil, files.WrapVfsError(err)
	}

	if dir != nil {
		return dir, nil, nil
	}
	return nil, file, nil
}

// Same as `loadDirOrFile` but intolerant of files, responds 422s
func loadDirFromParam(c echo.Context, inst *instance.Instance) (*vfs.DirDoc, error) {
	dir, file, err := loadDirOrFileFromParam(c, inst, permission.GET)
	if file != nil {
		return nil, jsonapi.InvalidParameter("file-id", errors.New("file-id: not a directory"))
	}
	return dir, err
}

// Same as `loadDirOrFile` but intolerant of directories, responds 422s
func loadFileFromParam(c echo.Context, inst *instance.Instance, perm permission.Verb) (*vfs.FileDoc, error) {
	dir, file, err := loadDirOrFileFromParam(c, inst, perm)
	if dir != nil {
		return nil, jsonapi.InvalidParameter("file-id", errors.New("file-id: not a file"))
	}
	return file, err
}

// HeadDirOrFile returns an error if the requested file or directory does not
// exist. It returns an empty body otherwise.
func HeadDirOrFile(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	_, _, err := loadDirOrFileFromParam(c, inst, permission.GET)
	if err != nil {
		return err
	}
	return nil
}

// ReadMetadataFromPath allows to get file/dir information for a path.
func ReadMetadataFromPath(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.ReadMetadataFromPath(c, s)
}

// GetDirOrFileData handles all GET requests on aiming at getting a file or
// directory metadata from its id.
// TODO: reuse files.ReadMetadataFromIDHandler?
func GetDirOrFileData(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	dir, file, err := loadDirOrFileFromParam(c, inst, permission.GET)
	if err != nil {
		return err
	}
	if dir != nil {
		return files.DirData(c, http.StatusOK, dir, s)
	}
	return files.FileData(c, http.StatusOK, file, true, nil, s)
}

// ReadFileContentFromIDHandler handles all GET requests aiming at downloading
// a file given its ID. It serves the file in inline mode.
// TODO: reuse files.ReadMetadataFromIDHandler?
func ReadFileContentFromIDHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	file, err := loadFileFromParam(c, inst, permission.GET)
	if err != nil {
		return err
	}
	return files.SendFileFromDoc(inst, c, file, false)
}

// ReadFileContentFromVersion handles the download of an old version of the
// file content.
func ReadFileContentFromVersion(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.ReadFileContentFromVersion(c)
}

// GetDirSize returns the size of a directory (the sum of the size of the files
// in this directory, including those in subdirectories).
func GetDirSize(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	fs := inst.VFS()
	dir, err := loadDirFromParam(c, inst)
	if err != nil {
		return err
	}

	size, err := fs.DirSize(dir)
	if err != nil {
		return files.WrapVfsError(err)
	}

	result := files.ApiDiskSize{DocID: dir.DocID, Size: size}
	return jsonapi.Data(c, http.StatusOK, &result, nil)
}

// ModifyMetadataByIDHandler handles PATCH requests used to modify the file or
// directory metadata, as well as moving and renaming it in the shared drive's filesystem.
func ModifyMetadataByIDHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	patch, err := getPatch(c, c.Param("file-id"))
	if err != nil {
		return files.WrapVfsError(err)
	}
	if err = applyPatch(c, inst.VFS(), patch); err != nil {
		return files.WrapVfsError(err)
	}
	return nil
}

func getPatch(c echo.Context, docID string) (*docPatch, error) {
	var patch docPatch
	obj, err := jsonapi.Bind(c.Request().Body, &patch)
	if err != nil {
		return nil, jsonapi.BadJSON()
	}
	patch.docID = docID
	patch.RestorePath = nil
	if rel, ok := obj.GetRelationship("parent"); ok {
		rid, ok := rel.ResourceIdentifier()
		if !ok {
			return nil, jsonapi.BadJSON()
		}
		patch.DirID = &rid.ID
	}
	return &patch, nil
}

func applyPatch(c echo.Context, fs vfs.VFS, patch *docPatch) (err error) {
	dir, file, err := fs.DirOrFileByID(patch.docID)
	if err != nil {
		return err
	}

	var rev string
	if dir != nil {
		rev = dir.Rev()
	} else {
		rev = file.Rev()
	}

	if err = files.CheckIfMatch(c, rev); err != nil {
		return err
	}

	if dir != nil {
		if err = middlewares.AllowVFS(c, permission.PATCH, dir); err != nil {
			return err
		}
	} else {
		if err = middlewares.AllowVFS(c, permission.PATCH, file); err != nil {
			return err
		}
	}

	if patch.DirID != nil {
		newParent, _, err := fs.DirOrFileByID(*patch.DirID)
		if err != nil {
			return err
		}
		if newParent == nil {
			return jsonapi.BadRequest(errors.New("destination directory does not exist"))
		}
		// XXX: This permission check ensures the new parent is in the shared drive.
		if err = middlewares.AllowVFS(c, permission.POST, newParent); err != nil {
			return err
		}
	}

	if dir != nil {
		files.UpdateDirCozyMetadata(c, dir)
		dir, err = vfs.ModifyDirMetadata(fs, dir, &patch.DocPatch)
		if err != nil {
			return err
		}
		if patch.Name != nil {
			// Update sharing description if this directory is a sharing root
			updateSharingDescriptionIfNeeded(c, dir)
		}
	} else {
		files.UpdateFileCozyMetadata(c, file, false)
		file, err = vfs.ModifyFileMetadata(fs, file, &patch.DocPatch)
		if err != nil {
			return err
		}
	}

	if dir != nil {
		return files.DirData(c, http.StatusOK, dir, nil)
	}
	return files.FileData(c, http.StatusOK, file, false, nil, nil)
}

// ChangesFeed is the handler for CouchDB's changes feed requests with some
// additional options, like skip_trashed.
//
// The feed will be filtered by the given sharing directory returning only
// files that are below this directory (anything outside this directory should
// only be deletions with the document ID as only information).
func ChangesFeed(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	// TODO: if owner then fail, shouldn't be accessing their own stuff, risk recursion download kinda thing
	// TODO: should this break if there ever is actually more than 1 directory ?
	sharedDir, err := getSharingDir(inst, s)
	if err != nil {
		return err
	}
	return files.ChangesFeed(c, inst, sharedDir)
}

// CopyFile copies a single file from a shared drive to itself using parameters
// from the echo Context:
// - url param: `file-id`: surce file's ID
// - url query param: `DirID`: optional destination folder's ID
// - url query param: `Name`: optional destination file name
func CopyFile(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.CopyFile(c, inst, s)
}

func CreationHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.Create(c, s)
}

// DestroyFileHandler handles DELETE requests to clear one element from the
// trash.
func DestroyFileHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.DestroyFileHandler(c)
}

// OverwriteFileContentHandler handles PUT requests to overwrite the content of
// a file given its identifier.
func OverwriteFileContentHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.OverwriteFileContent(c, s)
}

// RestoreTrashFileHandler handles POST requests to restore a file or directory
// from the trash.
func RestoreTrashFileHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.Restore(c, s)
}

// TrashHandler handles all DELETE requests to move the file or directory with
// the specified file-id to the trash.
func TrashHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.Trash(c, s)
}

// UploadMetadataHandler accepts a metadata objet and persists it, so that it
// can be used in a future file upload.
func UploadMetadataHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.UploadMetadataHandler(c)
}

// ThumbnailHandler serves thumbnails of the images/photos.
func ThumbnailHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.ThumbnailHandler(c)
}

// FileDownloadCreateHandler stores the required path into a secret usable for
// the download handler below.
func FileDownloadCreateHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.FileDownload(c, s)
}

// FileDownloadHandler sends the content of a file that has previously been
// prepared via a call to FileDownloadCreateHandler.
func FileDownloadHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.FileDownloadHandler(c)
}

// CreateNote allows to create a note inside a shared drive.
func CreateNote(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return notes.CreateNote(c)
}

// OpenNoteURL returns the parameters to open a note inside a shared drive.
func OpenNoteURL(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	s, err := sharing.FindSharing(inst, c.Param("id"))
	if err != nil {
		return wrapErrors(err)
	}
	if !s.Drive {
		return jsonapi.NotFound(errors.New("not a drive"))
	}
	if s.Owner {
		return notes.OpenNoteURL(c)
	}

	if err := middlewares.AllowWholeType(c, permission.GET, consts.Files); err != nil {
		return err
	}

	fileID := c.Param("file-id")
	fileOpener := &sharing.FileOpener{
		Inst:    inst,
		Sharing: s,
		File:    &vfs.FileDoc{DocID: fileID},
	}
	open := &sharing.NoteOpener{FileOpener: fileOpener}

	doc, err := open.GetResult(-1, false)
	if err != nil {
		return wrapErrors(err)
	}

	return jsonapi.Data(c, http.StatusOK, doc, nil)
}

// OpenOffice returns the parameter to open an office document inside a shared
// drive.
func OpenOffice(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	s, err := sharing.FindSharing(inst, c.Param("id"))
	if err != nil {
		return wrapErrors(err)
	}
	if !s.Drive {
		return jsonapi.NotFound(errors.New("not a drive"))
	}
	if s.Owner {
		return office.Open(c)
	}

	if err := middlewares.AllowWholeType(c, permission.GET, consts.Files); err != nil {
		return err
	}

	fileID := c.Param("file-id")
	fileOpener := &sharing.FileOpener{
		Inst:    inst,
		Sharing: s,
		File:    &vfs.FileDoc{DocID: fileID},
	}
	open := &sharing.OfficeOpener{FileOpener: fileOpener}

	doc, err := open.GetResult(-1, false)
	if err != nil {
		return wrapErrors(err)
	}

	return jsonapi.Data(c, http.StatusOK, doc, nil)
}

// getSharingDir returns the directory linked to the drive sharing.
func getSharingDir(inst *instance.Instance, s *sharing.Sharing) (*vfs.DirDoc, error) {
	dir, err := s.GetSharingDir(inst)
	if err != nil {
		return nil, jsonapi.NotFound(errors.New("shared drive not found"))
	}
	return dir, nil
}

// CreateSharedDrivePermissionHandler creates a share-by-link permission for a file in a shared drive.
func CreateSharedDrivePermissionHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	validate := func(perms permission.Set) error {
		return validateSharedDrivePermission(inst, s, perms)
	}

	return webperm.HandleCreateShareByLink(c, inst, validate, true)
}

// validateSharedDrivePermission checks that all file IDs in the permission rules are files/directories that belong to the shared drive.
func validateSharedDrivePermission(inst *instance.Instance, s *sharing.Sharing, perms permission.Set) error {
	rootDir, err := s.GetSharingDir(inst)
	if err != nil {
		return jsonapi.NotFound(errors.New("shared drive root directory not found"))
	}

	fs := inst.VFS()

	for _, perm := range perms {
		// Only allow exact io.cozy.files type - reject wildcards like "*" or "io.cozy.*"
		if perm.Type != consts.Files {
			return jsonapi.BadRequest(errors.New("shared drive permissions can only include files"))
		}
		// Reject empty values - share-by-link must target specific files
		if len(perm.Values) == 0 {
			return jsonapi.BadRequest(errors.New("shared drive permissions must specify file IDs"))
		}
		// Reject selectors - only file IDs are allowed
		if perm.Selector != "" {
			return jsonapi.BadRequest(errors.New("shared drive permissions cannot use selectors"))
		}
		for _, fileID := range perm.Values {
			// Check if the file/directory is within the shared drive
			if err := isWithinDirectory(fs, fileID, rootDir); err != nil {
				return jsonapi.BadRequest(errors.New("file is not within the shared drive"))
			}
		}
	}
	return nil
}

// isWithinDirectory checks if the given file/directory ID is within the
// specified parent directory (including the parent itself).
func isWithinDirectory(fs vfs.VFS, fileID string, parent *vfs.DirDoc) error {
	if fileID == parent.ID() {
		return nil
	}

	dir, file, err := fs.DirOrFileByID(fileID)
	if err != nil {
		return err
	}

	var fullpath string
	if dir != nil {
		fullpath = dir.Fullpath
	} else {
		fullpath, err = file.Path(fs)
		if err != nil {
			return err
		}
	}

	// Check if path is within parent directory
	if strings.HasPrefix(fullpath, parent.Fullpath+"/") {
		return nil
	}

	return errors.New("file is not within the directory")
}

// RevokeSharedDrivePermission revokes a share-by-link permission for a file in a shared drive.
func RevokeSharedDrivePermission(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	permID := c.Param("perm-id")
	if permID == "" {
		return jsonapi.BadRequest(errors.New("missing permission ID"))
	}

	perm, err := permission.GetPermissionByIDIncludingExpired(inst, permID)
	if err != nil {
		if couchdb.IsNotFoundError(err) {
			return jsonapi.NotFound(errors.New("permission not found"))
		}
		return err
	}

	if perm.Type != permission.TypeShareByLink {
		return jsonapi.BadRequest(errors.New("not a share-by-link permission"))
	}

	if err := validateSharedDrivePermission(inst, s, perm.Permissions); err != nil {
		return jsonapi.Forbidden(errors.New("permission does not belong to this shared drive"))
	}

	// Any member who can create share-by-link permissions can also revoke them
	if err := perm.Revoke(inst); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// drivesRoutes sets the routing for the shared drives
func drivesRoutes(router *echo.Group) {
	group := router.Group("/drives")
	group.GET("", ListSharedDrives)
	group.POST("", CreateSharedDrive)
	group.POST("/move", MoveHandler)

	drive := group.Group("/:id")

	drive.HEAD("/download/:file-id", proxy(ReadFileContentFromIDHandler, true))
	drive.GET("/download/:file-id", proxy(ReadFileContentFromIDHandler, true))

	drive.HEAD("/download/:file-id/:version-id", proxy(ReadFileContentFromVersion, true))
	drive.GET("/download/:file-id/:version-id", proxy(ReadFileContentFromVersion, true))

	drive.GET("/_changes", proxy(ChangesFeed, true))

	drive.HEAD("/:file-id", proxy(HeadDirOrFile, true))

	drive.GET("/metadata", proxy(ReadMetadataFromPath, true))
	drive.GET("/:file-id", proxy(GetDirOrFileData, true))
	drive.GET("/:file-id/size", proxy(GetDirSize, true))

	drive.PATCH("/:file-id", proxy(ModifyMetadataByIDHandler, true))

	drive.POST("/", proxy(CreationHandler, true))
	drive.POST("/:file-id", proxy(CreationHandler, true))
	drive.PUT("/:file-id", proxy(OverwriteFileContentHandler, true))
	drive.POST("/upload/metadata", proxy(UploadMetadataHandler, true))
	drive.POST("/:file-id/copy", proxy(CopyFile, true))

	drive.GET("/:file-id/thumbnails/:secret/:format", proxy(ThumbnailHandler, true))

	drive.POST("/downloads", proxy(FileDownloadCreateHandler, true))
	drive.GET("/downloads/:secret/:fake-name", proxy(FileDownloadHandler, false))

	drive.POST("/trash/:file-id", proxy(RestoreTrashFileHandler, true))
	drive.DELETE("/trash/:file-id", proxy(DestroyFileHandler, true))

	drive.DELETE("/:file-id", proxy(TrashHandler, true))

	drive.POST("/notes", proxy(CreateNote, true))
	drive.GET("/notes/:file-id/open", OpenNoteURL)
	drive.GET("/office/:file-id/open", OpenOffice)

	drive.GET("/realtime", Ws)

	// Share-by-link (public link) endpoints for files in shared drives
	drive.POST("/permissions", proxy(CreateSharedDrivePermissionHandler, true))
	drive.DELETE("/permissions/:perm-id", proxy(RevokeSharedDrivePermission, true))
}

func proxy(fn func(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error, needsAuth bool) echo.HandlerFunc {
	return func(c echo.Context) error {
		inst := middlewares.GetInstance(c)
		s, err := sharing.FindSharing(inst, c.Param("id"))
		if err != nil {
			return wrapErrors(err)
		}
		if !s.Drive {
			return jsonapi.NotFound(errors.New("not a drive"))
		}
		if !s.Active {
			return jsonapi.Forbidden(middlewares.ErrForbidden)
		}

		if s.Owner {
			return fn(c, inst, s)
		}

		// On a recipient, we proxy the request to the owner
		// Some routes need to be publicly accessible but others should be
		// require an authorization token.
		if needsAuth {
			method := c.Request().Method
			if method == http.MethodHead {
				method = http.MethodGet
			}
			verb := permission.Verb(method)
			if err := middlewares.AllowWholeType(c, verb, consts.Files); err != nil {
				return err
			}

			// For write operations, check if the user has read-only access
			// POST /downloads is a read operation (creates temporary download link)
			if method == http.MethodPost || method == http.MethodPut ||
				method == http.MethodPatch || method == http.MethodDelete {
				// Skip write check for download endpoint (read-only operation)
				path := c.Request().URL.Path
				isDownload := strings.HasSuffix(path, "/downloads")

				if !isDownload {
					_, err := checkSharedDrivePermission(inst, c.Param("id"), true)
					if err != nil {
						return err
					}
				}
			}
		}

		if len(s.Credentials) == 0 {
			return jsonapi.InternalServerError(errors.New("no credentials"))
		}
		token := s.Credentials[0].DriveToken
		u, err := url.Parse(s.Members[0].Instance)
		if err != nil {
			return jsonapi.InternalServerError(err)
		}

		// XXX Let's try to avoid one http request by cheating a bit. If the two
		// instances are on the same domain (same stack), we can call directly
		// the handler. We just need to fake the middlewares. It helps for
		// performances.
		if owner, err := lifecycle.GetInstance(u.Host); err == nil {
			pdoc, err := permission.GetForShareCode(owner, token)
			if err != nil {
				return err
			}
			middlewares.ForcePermission(c, pdoc)
			middlewares.SetInstance(c, owner)
			return fn(c, owner, s)
		}

		director := func(req *http.Request) {
			req.URL = u
			req.URL.Path = c.Request().URL.Path
			req.URL.RawQuery = c.Request().URL.RawQuery
			req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
			req.Header.Del(echo.HeaderCookie)
			req.Header.Del("Host")
		}
		proxy := &httputil.ReverseProxy{Director: director}
		logger := inst.Logger().WithNamespace("drive-proxy").Writer()
		defer logger.Close()
		proxy.ErrorLog = log.New(logger, "", 0)
		proxy.ServeHTTP(c.Response(), c.Request())
		return nil
	}
}

// checkSharedDrivePermission checks if the current user has permission to access
// the specified shared drive. It verifies that:
// 1. The sharing exists and is a drive
// 2. The current user is a member of the sharing (by domain or email)
// If requireWrite is true, it also checks that the user has write permission (not read-only).
// Returns the sharing if the user has the required permissions.
func checkSharedDrivePermission(inst *instance.Instance, sharingID string, requireWrite bool) (*sharing.Sharing, error) {
	// Find the sharing by ID
	s, err := sharing.FindSharing(inst, sharingID)
	if err != nil {
		// CouchDB not_found: treat as no access
		if strings.Contains(err.Error(), "not_found") {
			return nil, jsonapi.Forbidden(errors.New("not a member of this sharing"))
		}
		return nil, wrapErrors(err)
	}

	// Check if it's a drive
	if !s.Drive {
		return nil, jsonapi.NotFound(errors.New("not a drive"))
	}

	// Check that current user is a member of the sharing and get their read-only status
	currDomain := inst.Domain
	if strings.Contains(currDomain, ":") {
		currDomain = strings.SplitN(currDomain, ":", 2)[0]
	}

	// Get current instance email for comparison
	currEmail, _ := inst.SettingsEMail()

	isMember := false
	isReadOnly := false

	// If this is the owner instance, they're a member with write access
	if s.Owner {
		isMember = true
		isReadOnly = false
	} else {
		// On a recipient's instance, their own member entry is typically at index 1
		// (index 0 is the owner). Check if there's a member with an Instance field set.
		for _, m := range s.Members {
			memberHost := m.InstanceHost()
			if memberHost == "" {
				continue
			}
			// Check by domain
			if memberHost == inst.Domain || memberHost == currDomain {
				isMember = true
				isReadOnly = m.ReadOnly
				break
			}
			// Check by email
			if currEmail != "" && m.Email == currEmail {
				isMember = true
				isReadOnly = m.ReadOnly
				break
			}
		}
	}

	if !isMember {
		return nil, jsonapi.Forbidden(errors.New("not a member of this sharing"))
	}

	// If write permission is required, check that the user is not read-only
	if requireWrite && isReadOnly {
		return nil, jsonapi.Forbidden(errors.New("write access denied: read-only member"))
	}

	return s, nil
}

// updateSharingDescriptionIfNeeded checks if the given directory is a sharing
// root and triggers a job to update the sharing description if needed.
func updateSharingDescriptionIfNeeded(c echo.Context, dir *vfs.DirDoc) {
	inst := middlewares.GetInstance(c)

	// Check if this directory is referenced by any sharings
	for _, ref := range dir.ReferencedBy {
		if ref.Type == consts.Sharings {
			// This directory is a sharing root, trigger an update job
			msg, err := job.NewMessage(&sharing.UpdateMsg{
				SharingID:      ref.ID,
				NewDescription: dir.DocName,
			})
			if err != nil {
				inst.Logger().WithNamespace("sharing").
					Warnf("Failed to create share-update message: %s", err)
				continue
			}

			_, err = job.System().PushJob(inst, &job.JobRequest{
				WorkerType: "share-update",
				Message:    msg,
			})
			if err != nil {
				inst.Logger().WithNamespace("sharing").
					Warnf("Failed to push share-update job for sharing %s: %s", ref.ID, err)
			}
		}
	}
}
