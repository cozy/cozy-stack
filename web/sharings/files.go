package sharings

import (
	"io"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web/files"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

// TODO Support sharing of recursive directories. For now all directories go
// to /Shared With Me/
func creationWithIDHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	fs := instance.VFS()

	_, err := fs.DirByID(consts.SharedWithMeDirID)
	if err != nil {
		err = createSharedWithMeDir(fs)
		if err != nil {
			return err
		}
	}

	switch c.QueryParam("Type") {
	case consts.FileType:
		err = createFileWithIDHandler(c, fs)
	case consts.DirType:
		err = createDirWithIDHandler(c, fs)
	default:
		return files.ErrDocTypeInvalid
	}

	return err
}

func createDirWithIDHandler(c echo.Context, fs vfs.VFS) error {
	name := c.QueryParam("Name")
	id := c.Param("docid")
	date := c.Request().Header.Get("Date")

	// TODO handle name collision.
	doc, err := vfs.NewDirDoc(fs, name, "", nil)
	if err != nil {
		return err
	}

	doc.DirID = consts.SharedWithMeDirID
	doc.SetID(id)

	if date != "" {
		if t, errt := time.Parse(time.RFC1123, date); errt == nil {
			doc.CreatedAt = t
			doc.UpdatedAt = t
		}
	}

	if err = permissions.AllowVFS(c, "POST", doc); err != nil {
		return err
	}

	return fs.CreateDir(doc)
}

func createFileWithIDHandler(c echo.Context, fs vfs.VFS) error {
	name := c.QueryParam("Name")

	doc, err := files.FileDocFromReq(c, name, "", nil)
	if err != nil {
		return err
	}

	doc.SetID(c.Param("docid"))
	doc.DirID = consts.SharedWithMeDirID

	if err = permissions.AllowVFS(c, "POST", doc); err != nil {
		return err
	}

	file, err := fs.CreateFile(doc, nil)
	if err != nil {
		return err
	}

	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	_, err = io.Copy(file, c.Request().Body)
	return err
}

func createSharedWithMeDir(fs vfs.VFS) error {
	// TODO Put "Shared With Me" in a local-aware constant.
	dirDoc, err := vfs.NewDirDoc(fs, "Shared With Me", "", nil)
	if err != nil {
		return err
	}

	dirDoc.SetID(consts.SharedWithMeDirID)
	t := time.Now()
	dirDoc.CreatedAt = t
	dirDoc.UpdatedAt = t

	return fs.CreateDir(dirDoc)
}
