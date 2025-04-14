package sharings

import (
	"errors"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/files"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

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

// Load either a DirDoc or a FileDoc from the given `file-id` param. The function also checks permissions
func loadDirOrFileFromParam(c echo.Context, inst *instance.Instance, s *sharing.Sharing) (*vfs.DirDoc, *vfs.FileDoc, error) {
	dir, file, err := inst.VFS().DirOrFileByID(c.Param("file-id"))
	if err != nil {
		return nil, nil, files.WrapVfsError(err)
	}

	if dir != nil {
		err = middlewares.AllowVFS(c, permission.GET, dir)
	} else {
		err = middlewares.AllowVFS(c, permission.GET, file)
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
func loadDirFromParam(c echo.Context, inst *instance.Instance, s *sharing.Sharing) (*vfs.DirDoc, error) {
	dir, file, err := loadDirOrFileFromParam(c, inst, s)
	if file != nil {
		return nil, jsonapi.InvalidParameter("file-id", errors.New("file-id: not a directory"))
	}
	return dir, err
}

// Same as `loadDirOrFile` but intolerant of directories, responds 422s
func loadFileFromParam(c echo.Context, inst *instance.Instance, s *sharing.Sharing) (*vfs.FileDoc, error) {
	dir, file, err := loadDirOrFileFromParam(c, inst, s)
	if dir != nil {
		return nil, jsonapi.InvalidParameter("file-id", errors.New("file-id: not a file"))
	}
	return file, err
}

func HeadDirOrFile(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	_, _, err := loadDirOrFileFromParam(c, inst, s)
	if err != nil {
		return err
	}
	return nil
}

func GetDirOrFileData(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	dir, file, err := loadDirOrFileFromParam(c, inst, s)
	if err != nil {
		return err
	}
	if dir != nil {
		return files.DirData(c, http.StatusOK, dir)
	}
	return files.FileData(c, http.StatusOK, file, true, nil)
}

func DownloadFile(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	//TODO: CSP ?
	file, err := loadFileFromParam(c, inst, s)
	if err != nil {
		return err
	}
	//TODO: 403 if check perms is true
	return files.SendFileFromDoc(inst, c, file, false)
}

func GetDirSize(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	fs := inst.VFS()
	dir, err := loadDirFromParam(c, inst, s)
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

func CopyFile(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.CopyFile(c, inst)
}

// From the provided drive sharing, find the linked directories, and check VFS
// permission for the user in the context. If any fail, return an error.
func resolveSharingToPermittedDirIDs(c echo.Context, inst *instance.Instance, s *sharing.Sharing) ([]string, error) {
	fs := inst.VFS()
	var directoryIDsOfSharing []string
	for _, rule := range s.Rules {
		if rule.DocType != consts.Files || (rule.Selector != "" && rule.Selector != "id") {
			continue
		}
		for _, dirID := range rule.Values {
			dir, err := fs.DirByID(dirID)
			if err != nil {
				return nil, files.WrapVfsError(err)
			}
			// TODO: this is all the security there is for this endpoint, please triple check and remove this comment
			if err := middlewares.AllowVFS(c, permission.GET, dir); err != nil {
				return nil, err
			}
		}
		directoryIDsOfSharing = append(directoryIDsOfSharing, rule.Values...)
	}

	if len(directoryIDsOfSharing) == 0 {
		return nil, errors.New("sharing has no matching directories")
	}
	return directoryIDsOfSharing, nil
}

func ChangesFeed(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	// TODO: if owner then fail, shouldn't be accessing their own stuff, risk recursion download kinda thing
	// TODO: should this break if there ever is actually more than 1 directory ?
	// TODO: consider nested sharings
	directoryIDsOfSharing, err := resolveSharingToPermittedDirIDs(c, inst, s)
	if err != nil {
		return err
	}
	return files.ChangesFeed(c, s.ID(), directoryIDsOfSharing)
}

// drivesRoutes sets the routing for the shared drives
func drivesRoutes(router *echo.Group) {
	group := router.Group("/drives")
	group.GET("", ListSharedDrives)

	drive := group.Group("/:id")
	drive.GET("/_changes", proxy(ChangesFeed))
	drive.HEAD("/:file-id", proxy(HeadDirOrFile))
	drive.GET("/:file-id", proxy(GetDirOrFileData))
	drive.GET("/:file-id/download", proxy(DownloadFile))
	drive.GET("/:file-id/size", proxy(GetDirSize))
	drive.POST("/:file-id/copy", proxy(CopyFile))
}

func proxy(fn func(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error) echo.HandlerFunc {
	return func(c echo.Context) error {
		inst := middlewares.GetInstance(c)
		s, err := sharing.FindSharing(inst, c.Param("id"))
		if err != nil {
			return wrapErrors(err)
		}
		if !s.Drive {
			return jsonapi.NotFound(errors.New("not a drive"))
		}

		if s.Owner {
			return fn(c, inst, s)
		}

		// On a recipient, we proxy the request to the owner
		method := c.Request().Method
		if method == http.MethodHead {
			method = http.MethodGet
		}
		verb := permission.Verb(method)
		if err := middlewares.AllowWholeType(c, verb, consts.Files); err != nil {
			return err
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
			s, err := sharing.FindSharing(owner, c.Param("id"))
			if err != nil {
				return wrapErrors(err)
			}
			pdoc, err := permission.GetForShareCode(owner, token)
			if err != nil {
				return err
			}
			middlewares.ForcePermission(c, pdoc)
			return fn(c, owner, s)
		}

		director := func(req *http.Request) {
			req.URL = u
			req.URL.Path = c.Request().URL.Path
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
