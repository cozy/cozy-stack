package vfsswift

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/swift"
)

var unixEpochZero = time.Time{}

// NewThumbsFsV2 creates a new thumb filesystem base on swift.
//
// This version stores the thumbnails in the same container as the main data
// container.
func NewThumbsFsV2(c *swift.Connection, domain string) vfs.Thumbser {
	return &thumbsV2{c: c, container: swiftV2ContainerPrefix + domain}
}

type thumbsV2 struct {
	c         *swift.Connection
	container string
}

func (t *thumbsV2) CreateThumb(img *vfs.FileDoc, format string) (io.WriteCloser, error) {
	return t.c.ObjectCreate(t.container, t.makeName(img, format), false, "", "", nil)
}

func (t *thumbsV2) RemoveThumbs(img *vfs.FileDoc, formats []string) error {
	objNames := make([]string, len(formats))
	for i, format := range formats {
		objNames[i] = t.makeName(img, format)
	}
	_, err := t.c.BulkDelete(t.container, objNames)
	return err
}

func (t *thumbsV2) ServeThumbContent(w http.ResponseWriter, req *http.Request, img *vfs.FileDoc, format string) error {
	name := t.makeName(img, format)
	f, o, err := t.c.ObjectOpen(t.container, name, false, nil)
	if err != nil {
		return wrapSwiftErr(err)
	}
	defer f.Close()

	w.Header().Set("Etag", fmt.Sprintf(`"%s"`, o["Etag"]))
	http.ServeContent(w, req, name, unixEpochZero, f)
	return nil
}

func (t *thumbsV2) makeName(img *vfs.FileDoc, format string) string {
	return fmt.Sprintf("thumbs/%s-%s", img.ID(), format)
}
