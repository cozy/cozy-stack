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

func HeadDirOrFile(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	dir, file, err := inst.VFS().DirOrFileByID(c.Param("file-id"))
	if err != nil {
		return files.WrapVfsError(err)
	}

	if dir != nil {
		err = middlewares.AllowVFS(c, permission.GET, dir)
	} else {
		err = middlewares.AllowVFS(c, permission.GET, file)
	}
	if err != nil {
		return err
	}
	return nil
}

func GetDirOrFile(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	dir, file, err := inst.VFS().DirOrFileByID(c.Param("file-id"))
	if err != nil {
		return files.WrapVfsError(err)
	}

	if dir != nil {
		err = middlewares.AllowVFS(c, permission.GET, dir)
	} else {
		err = middlewares.AllowVFS(c, permission.GET, file)
	}
	if err != nil {
		return err
	}

	if dir != nil {
		return files.DirData(c, http.StatusOK, dir)
	}
	return files.FileData(c, http.StatusOK, file, true, nil)
}

func GetDirSize(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	fs := inst.VFS()
	fileID := c.Param("file-id")
	dir, err := fs.DirByID(fileID)
	if err != nil {
		return files.WrapVfsError(err)
	}

	if err := middlewares.AllowVFS(c, permission.GET, dir); err != nil {
		return err
	}

	size, err := fs.DirSize(dir)
	if err != nil {
		return files.WrapVfsError(err)
	}

	result := files.ApiDiskSize{DocID: fileID, Size: size}
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
	return files.ChangesFeed(c, directoryIDsOfSharing)
}

// drivesRoutes sets the routing for the shared drives
func drivesRoutes(router *echo.Group) {
	group := router.Group("/drives")
	group.GET("", ListSharedDrives)

	drive := group.Group("/:id")
	drive.GET("/_changes", proxy(ChangesFeed))
	drive.HEAD("/:file-id", proxy(HeadDirOrFile))
	drive.GET("/:file-id", proxy(GetDirOrFile))
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
