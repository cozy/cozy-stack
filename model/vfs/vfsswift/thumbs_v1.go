package vfsswift

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/ncw/swift"
)

// NewThumbsFs creates a new thumb filesystem base on swift.
func NewThumbsFs(c *swift.Connection, domain string) vfs.Thumbser {
	return &thumbs{c: c, container: swiftV1DataContainerPrefix + domain}
}

type thumbs struct {
	c         *swift.Connection
	container string
}

func (t *thumbs) CreateThumb(img *vfs.FileDoc, format string) (vfs.ThumbFiler, error) {
	if err := t.c.ContainerCreate(t.container, nil); err != nil {
		return nil, err
	}
	name := t.makeName(img.ID(), format)
	obj, err := t.c.ObjectCreate(t.container, name, true, "", "", nil)
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
	name := t.makeName(img.ID(), format)
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
		objNames[i] = t.makeName(img.ID(), format)
	}
	_, err := t.c.BulkDelete(t.container, objNames)
	return err
}

func (t *thumbs) ServeThumbContent(w http.ResponseWriter, req *http.Request, img *vfs.FileDoc, format string) error {
	name := t.makeName(img.ID(), format)
	f, o, err := t.c.ObjectOpen(t.container, name, false, nil)
	if err != nil {
		return wrapSwiftErr(err)
	}
	defer f.Close()

	lastModified, _ := time.Parse(http.TimeFormat, o["Last-Modified"])
	w.Header().Set("Etag", fmt.Sprintf(`"%s"`, o["Etag"]))

	http.ServeContent(w, req, name, lastModified, f)
	return nil
}

func (t *thumbs) CreateNoteThumb(id, mime string) (vfs.ThumbFiler, error) {
	if err := t.c.ContainerCreate(t.container, nil); err != nil {
		return nil, err
	}
	name := t.makeName(id, noteThumbFormat)
	obj, err := t.c.ObjectCreate(t.container, name, true, "", "", nil)
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

func (t *thumbs) OpenNoteThumb(id string) (io.ReadCloser, error) {
	name := t.makeName(id, noteThumbFormat)
	obj, _, err := t.c.ObjectOpen(t.container, name, false, nil)
	if err == swift.ObjectNotFound {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func (t *thumbs) RemoveNoteThumb(id string) error {
	objName := t.makeName(id, noteThumbFormat)
	return t.c.ObjectDelete(t.container, objName)
}

func (t *thumbs) ServeNoteThumbContent(w http.ResponseWriter, req *http.Request, id string) error {
	name := t.makeName(id, noteThumbFormat)
	f, o, err := t.c.ObjectOpen(t.container, name, false, nil)
	if err != nil {
		return wrapSwiftErr(err)
	}
	defer f.Close()

	lastModified, _ := time.Parse(http.TimeFormat, o["Last-Modified"])
	w.Header().Set("Etag", fmt.Sprintf(`"%s"`, o["Etag"]))

	http.ServeContent(w, req, name, lastModified, f)
	return nil
}

func (t *thumbs) makeName(imgID string, format string) string {
	return fmt.Sprintf("thumbs/%s-%s", imgID, format)
}

func wrapSwiftErr(err error) error {
	if err == swift.ObjectNotFound || err == swift.ContainerNotFound {
		return os.ErrNotExist
	}
	return err
}
