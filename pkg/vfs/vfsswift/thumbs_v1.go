package vfsswift

import (
	"fmt"
	"net/http"
	"os"

	"github.com/cozy/cozy-stack/pkg/vfs"
	webutils "github.com/cozy/cozy-stack/web/utils"
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

func (t *thumbs) CreateThumb(img *vfs.FileDoc, format string) (vfs.ThumbFiler, error) {
	// TODO(optim): proper initialization of the container to avoir having to
	// recreate it every time.
	if err := t.c.ContainerCreate(t.container, nil); err != nil {
		return nil, err
	}
	name := t.makeName(img, format)
	obj, err := t.c.ObjectCreate(t.container, name, false, "", "", nil)
	if err != nil {
		return nil, err
	}
	th := &thumb{
		WriteCloser: obj,
		c:           t.c,
		container:   t.container,
		name:        name,
	}
	return th, nil
}

func (t *thumbs) ThumbExists(img *vfs.FileDoc, format string) (bool, error) {
	name := t.makeName(img, format)
	infos, _, err := t.c.Object(t.container, name)
	if err == swift.ObjectNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return infos.Bytes > 0, nil
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
	size, err := f.Length()
	if err != nil {
		return err
	}
	eTag := o["Etag"]
	if !webutils.CheckPreconditions(w, req, eTag) {
		return nil
	}
	webutils.ServeContent(w, req, "image/jpeg", size, f)
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
