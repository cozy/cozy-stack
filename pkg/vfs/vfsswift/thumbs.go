package vfsswift

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/swift"
)

// NewThumbsFs creates a new thumb filesystem base on swift.
func NewThumbsFs(c *swift.Connection, domain string) vfs.Thumbser {
	return &thumbs{c: c, container: "data-" + domain}
}

type thumbs struct {
	c         *swift.Connection
	container string
}

func (t *thumbs) CreateThumb(img *vfs.FileDoc, format string) (io.WriteCloser, error) {
	// TODO(optim): proper initialization of the container to avoir having to
	// recreate it every time.
	if err := t.c.ContainerCreate(t.container, nil); err != nil {
		return nil, err
	}
	return t.c.ObjectCreate(t.container, t.makeName(img, format), false, "", "", nil)
}

func (t *thumbs) RemoveThumbs(img *vfs.FileDoc, formats []string) error {
	objNames := make([]string, len(formats))
	for i, format := range formats {
		objNames[i] = t.makeName(img, format)
	}
	_, err := t.c.BulkDelete(t.container, objNames)
	return err
}

func (t *thumbs) ServeThumbContent(w http.ResponseWriter, req *http.Request, img *vfs.FileDoc, format string) error {
	name := t.makeName(img, format)
	f, o, err := t.c.ObjectOpen(t.container, name, false, nil)
	if err != nil {
		return wrapSwiftErr(err)
	}
	defer f.Close()

	lastModified, _ := time.Parse(http.TimeFormat, o["Last-Modified"]) // #nosec
	w.Header().Set("Etag", fmt.Sprintf(`"%s"`, o["Etag"]))

	http.ServeContent(w, req, name, lastModified, f)
	return nil
}

func (t *thumbs) makeName(img *vfs.FileDoc, format string) string {
	return fmt.Sprintf("thumbs/%s-%s", img.ID(), format)
}

func wrapSwiftErr(err error) error {
	if err == swift.ObjectNotFound || err == swift.ContainerNotFound {
		return os.ErrNotExist
	}
	return err
}
