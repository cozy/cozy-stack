package move

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/cozy/afero"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/crypto"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/ncw/swift/v2"
)

var (
	archiveMaxAge    = 7 * 24 * time.Hour
	archiveMACConfig = crypto.MACConfig{
		Name:   "exports",
		MaxAge: archiveMaxAge,
		MaxLen: 256,
	}
)

// Archiver is an interface describing an abstraction for storing archived
// data.
type Archiver interface {
	OpenArchive(inst *instance.Instance, exportDoc *ExportDoc) (io.ReadCloser, error)
	CreateArchive(exportDoc *ExportDoc) (io.WriteCloser, error)
	RemoveArchives(exportDocs []*ExportDoc) error
}

// SystemArchiver returns the global system archiver, corresponding to the
// user's configuration.
func SystemArchiver() Archiver {
	fsURL := config.FsURL()
	switch fsURL.Scheme {
	case config.SchemeFile, config.SchemeMem:
		fs := afero.NewBasePathFs(afero.NewOsFs(), path.Join(fsURL.Path, "exports"))
		return newAferoArchiver(fs)
	case config.SchemeSwift, config.SchemeSwiftSecure:
		return newSwiftArchiver()
	default:
		panic(fmt.Errorf("exports: unknown storage provider %s", fsURL.Scheme))
	}
}

func newAferoArchiver(fs afero.Fs) Archiver {
	return aferoArchiver{fs}
}

type aferoArchiver struct {
	fs afero.Fs
}

func (a aferoArchiver) fileName(exportDoc *ExportDoc) string {
	return path.Join(exportDoc.Domain, exportDoc.ID()+"tar.gz")
}

func (a aferoArchiver) OpenArchive(inst *instance.Instance, exportDoc *ExportDoc) (io.ReadCloser, error) {
	return a.fs.Open(a.fileName(exportDoc))
}

func (a aferoArchiver) CreateArchive(exportDoc *ExportDoc) (io.WriteCloser, error) {
	f, err := a.fs.OpenFile(a.fileName(exportDoc), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if os.IsNotExist(err) {
		if err = a.fs.MkdirAll(path.Join("/", exportDoc.Domain), 0700); err == nil {
			f, err = a.fs.OpenFile(a.fileName(exportDoc), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		}
	}
	return f, err
}

func (a aferoArchiver) RemoveArchives(exportDocs []*ExportDoc) error {
	var errm error
	for _, e := range exportDocs {
		if err := a.fs.Remove(a.fileName(e)); err != nil {
			errm = multierror.Append(errm, err)
		}
	}
	return errm
}

func newSwiftArchiver() Archiver {
	return &switfArchiver{
		c:         config.GetSwiftConnection(),
		container: "exports",
		ctx:       context.Background(),
	}
}

type switfArchiver struct {
	c         *swift.Connection
	container string
	ctx       context.Context
}

func (a *switfArchiver) init() error {
	if _, _, err := a.c.Container(a.ctx, a.container); err == swift.ContainerNotFound {
		if err = a.c.ContainerCreate(a.ctx, a.container, nil); err != nil {
			return err
		}
	}
	return nil
}

func (a *switfArchiver) OpenArchive(inst *instance.Instance, exportDoc *ExportDoc) (io.ReadCloser, error) {
	if err := a.init(); err != nil {
		return nil, err
	}
	objectName := exportDoc.Domain + "/" + exportDoc.ID()
	f, _, err := a.c.ObjectOpen(a.ctx, a.container, objectName, false, nil)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (a *switfArchiver) CreateArchive(exportDoc *ExportDoc) (io.WriteCloser, error) {
	if err := a.init(); err != nil {
		return nil, err
	}
	objectName := exportDoc.Domain + "/" + exportDoc.ID()
	objectMeta := swift.Metadata{
		"created-at": exportDoc.CreatedAt.Format(time.RFC3339),
	}
	headers := objectMeta.ObjectHeaders()
	headers["X-Delete-At"] = strconv.FormatInt(exportDoc.ExpiresAt.Unix(), 10)
	return a.c.ObjectCreate(a.ctx, a.container, objectName, true, "",
		"application/tar+gzip", headers)
}

func (a *switfArchiver) RemoveArchives(exportDocs []*ExportDoc) error {
	if err := a.init(); err != nil {
		return err
	}
	var objectNames []string
	for _, e := range exportDocs {
		objectNames = append(objectNames, e.Domain+"/"+e.ID())
	}
	if len(objectNames) > 0 {
		_, err := a.c.BulkDelete(a.ctx, a.container, objectNames)
		return err
	}
	return nil
}
