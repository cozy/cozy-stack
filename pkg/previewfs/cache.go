package previewfs

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/ncw/swift"
	"github.com/spf13/afero"
)

const (
	containerName = "previews"
	ttl           = 30 * 24 * time.Hour
)

// Cache is a interface for persisting previews of PDF for later reuse.
type Cache interface {
	Get(md5sum []byte) (*bytes.Buffer, error)
	Set(md5sum []byte, buffer *bytes.Buffer) error
}

// SystemCache returns the global cache, using the configuration file.
func SystemCache() Cache {
	fsURL := config.FsURL()
	switch fsURL.Scheme {
	case config.SchemeFile, config.SchemeMem:
		fs := afero.NewBasePathFs(afero.NewOsFs(), path.Join(fsURL.Path, containerName))
		return aferoCache{fs}
	case config.SchemeSwift, config.SchemeSwiftSecure:
		conn := config.GetSwiftConnection()
		return swiftCache{conn}
	default:
		panic(fmt.Errorf("previewfs: unknown storage provider %s", fsURL.Scheme))
	}
}

type aferoCache struct {
	fs afero.Fs
}

func (a aferoCache) Get(md5sum []byte) (*bytes.Buffer, error) {
	f, err := a.fs.Open(filename(md5sum))
	if err != nil {
		return nil, err
	}
	return readClose(f)
}

func (a aferoCache) Set(md5sum []byte, buffer *bytes.Buffer) error {
	exists, err := afero.DirExists(a.fs, "/")
	if err != nil || !exists {
		_ = a.fs.MkdirAll("/", 0700)
	}
	f, err := a.fs.OpenFile(filename(md5sum), os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	return writeClose(f, buffer)
}

type swiftCache struct {
	c *swift.Connection
}

func (s swiftCache) Get(md5sum []byte) (*bytes.Buffer, error) {
	f, _, err := s.c.ObjectOpen(containerName, filename(md5sum), false, nil)
	if err != nil {
		return nil, err
	}
	return readClose(f)
}

func (s swiftCache) Set(md5sum []byte, buffer *bytes.Buffer) error {
	objectName := filename(md5sum)
	objectMeta := swift.Metadata{"created-at": time.Now().Format(time.RFC3339)}
	headers := objectMeta.ObjectHeaders()
	headers["X-Delete-After"] = strconv.FormatInt(int64(ttl.Seconds()), 10)
	f, err := s.c.ObjectCreate(containerName, objectName, true, "", "image/jpg", headers)
	if err == swift.ObjectNotFound {
		_ = s.c.ContainerCreate(containerName, nil)
		f, err = s.c.ObjectCreate(containerName, objectName, true, "", "image/jpg", headers)
	}
	if err != nil {
		return err
	}
	return writeClose(f, buffer)
}

func filename(md5sum []byte) string {
	return hex.EncodeToString(md5sum) + ".jpg"
}

func readClose(f io.ReadCloser) (*bytes.Buffer, error) {
	buffer := &bytes.Buffer{}
	_, err := buffer.ReadFrom(f)
	if errc := f.Close(); errc != nil && err == nil {
		return nil, errc
	}
	return buffer, err
}

func writeClose(f io.WriteCloser, buffer *bytes.Buffer) error {
	_, err := f.Write(buffer.Bytes())
	if errc := f.Close(); errc != nil && err == nil {
		err = errc
	}
	return err
}
