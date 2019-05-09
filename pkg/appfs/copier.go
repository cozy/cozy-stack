package appfs

import (
	"compress/gzip"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/afero"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/filetype"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/swift"
)

// Copier is an interface defining a common set of functions for the installer
// to copy the application into an unknown storage.
type Copier interface {
	Start(slug, version, shasum string) (exists bool, err error)
	Copy(stat os.FileInfo, src io.Reader) error
	Abort() error
	Commit() error
}

type swiftCopier struct {
	c         *swift.Connection
	appObj    string
	tmpObj    string
	container string
	started   bool
}

type aferoCopier struct {
	fs      afero.Fs
	appDir  string
	tmpDir  string
	started bool
}

// NewSwiftCopier defines a Copier storing data into a swift container.
func NewSwiftCopier(conn *swift.Connection, appsType consts.AppType) Copier {
	return &swiftCopier{
		c:         conn,
		container: containerName(appsType),
	}
}

func (f *swiftCopier) Start(slug, version, shasum string) (bool, error) {
	f.appObj = path.Join(slug, version)
	if shasum != "" {
		f.appObj += "-" + shasum
	}
	_, _, err := f.c.Object(f.container, f.appObj)
	if err == nil {
		return true, nil
	}
	if err != swift.ObjectNotFound {
		return false, err
	}
	if _, _, err = f.c.Container(f.container); err == swift.ContainerNotFound {
		if err = f.c.ContainerCreate(f.container, nil); err != nil {
			return false, err
		}
	}
	f.tmpObj = "tmp-" + utils.RandomString(20) + "/"
	f.started = true
	return false, err
}

func (f *swiftCopier) Copy(stat os.FileInfo, src io.Reader) (err error) {
	if !f.started {
		panic("copier should call Start() before Copy()")
	}

	objName := path.Join(f.tmpObj, stat.Name())
	objMeta := swift.Metadata{
		"content-encoding":        "gzip",
		"original-content-length": strconv.FormatInt(stat.Size(), 10),
	}

	contentType := filetype.ByExtension(path.Ext(stat.Name()))
	if contentType == "" {
		contentType, src = filetype.FromReader(src)
	}

	file, err := f.c.ObjectCreate(f.container, objName, true, "",
		contentType, objMeta.ObjectHeaders())
	if err != nil {
		return err
	}
	defer func() {
		if errc := file.Close(); errc != nil {
			err = errc
		}
	}()

	gw, err := gzip.NewWriterLevel(file, gzip.BestCompression)
	if err != nil {
		return err
	}
	defer func() {
		if errc := gw.Close(); errc != nil && err == nil {
			err = errc
		}
	}()

	_, err = io.Copy(gw, src)
	return err
}

func (f *swiftCopier) Abort() error {
	objectNames, err := f.c.ObjectNamesAll(f.container, &swift.ObjectsOpts{
		Prefix:  f.tmpObj,
		Headers: swift.Headers{"X-Newest": "true"},
	})
	if err != nil {
		return err
	}
	_, err = f.c.BulkDelete(f.container, objectNames)
	return err
}

func (f *swiftCopier) Commit() (err error) {
	objectNames, err := f.c.ObjectNamesAll(f.container, &swift.ObjectsOpts{
		Prefix: f.tmpObj,
	})
	if err != nil {
		return err
	}
	defer func() {
		_, errc := f.c.BulkDelete(f.container, objectNames)
		if errc != nil {
			logger.WithNamespace("appfs").Errorf("Cannot BulkDelete after commit: %s", errc)
		}
	}()
	// We check if the appObj has not been created concurrently by another
	// copier.
	_, _, err = f.c.Object(f.container, f.appObj)
	if err == nil {
		return nil
	}
	for _, srcObjectName := range objectNames {
		dstObjectName := path.Join(f.appObj, strings.TrimPrefix(srcObjectName, f.tmpObj))
		_, err = f.c.ObjectCopy(f.container, srcObjectName, f.container, dstObjectName, nil)
		if err != nil {
			return err
		}
	}
	return f.c.ObjectPutString(f.container, f.appObj, "", "text/plain")
}

// NewAferoCopier defines a copier using an afero.Fs filesystem to store the
// application data.
func NewAferoCopier(fs afero.Fs) Copier {
	return &aferoCopier{fs: fs}
}

func (f *aferoCopier) Start(slug, version, shasum string) (bool, error) {
	f.appDir = path.Join("/", slug, version)
	if shasum != "" {
		f.appDir += "-" + shasum
	}
	exists, err := afero.DirExists(f.fs, f.appDir)
	if err != nil || exists {
		return exists, err
	}
	dir := path.Dir(f.appDir)
	if err = f.fs.MkdirAll(dir, 0755); err != nil {
		return false, err
	}
	f.tmpDir, err = afero.TempDir(f.fs, dir, "tmp")
	if err != nil {
		return false, err
	}
	f.started = true
	return false, nil
}

func (f *aferoCopier) Copy(stat os.FileInfo, src io.Reader) (err error) {
	if !f.started {
		panic("copier should call Start() before Copy()")
	}

	fullpath := path.Join(f.tmpDir, stat.Name()) + ".gz"
	dir := path.Dir(fullpath)
	if err = f.fs.MkdirAll(dir, 0755); err != nil {
		return err
	}

	dst, err := f.fs.Create(fullpath)
	if err != nil {
		return err
	}
	defer func() {
		if errc := dst.Close(); errc != nil {
			err = errc
		}
	}()

	gw, err := gzip.NewWriterLevel(dst, gzip.BestCompression)
	if err != nil {
		return err
	}
	defer func() {
		if errc := gw.Close(); errc != nil && err == nil {
			err = errc
		}
	}()

	_, err = io.Copy(gw, src)
	return err
}

func (f *aferoCopier) Commit() error {
	return f.fs.Rename(f.tmpDir, f.appDir)
}

func (f *aferoCopier) Abort() error {
	return f.fs.RemoveAll(f.tmpDir)
}

// NewFileInfo returns an os.FileInfo
func NewFileInfo(name string, size int64, mode os.FileMode) os.FileInfo {
	return &fileInfo{
		name: name,
		size: size,
		mode: mode,
	}
}

type fileInfo struct {
	name string
	size int64
	mode os.FileMode
}

func (f *fileInfo) Name() string       { return f.name }
func (f *fileInfo) Size() int64        { return f.size }
func (f *fileInfo) Mode() os.FileMode  { return f.mode }
func (f *fileInfo) ModTime() time.Time { return time.Now() }
func (f *fileInfo) IsDir() bool        { return false }
func (f *fileInfo) Sys() interface{}   { return nil }
