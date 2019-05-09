package vfsswift

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/swift"
)

var unixEpochZero = time.Time{}

// NewThumbsFsV2 creates a new thumb filesystem base on swift.
//
// This version stores the thumbnails in the same container as the main data
// container.
func NewThumbsFsV2(c *swift.Connection, db prefixer.Prefixer) vfs.Thumbser {
	return &thumbsV2{c: c, container: swiftV2ContainerPrefixData + db.DBPrefix()}
}

type thumbsV2 struct {
	c         *swift.Connection
	container string
}

type thumb struct {
	io.WriteCloser
	c         *swift.Connection
	container string
	name      string
}

func (t *thumb) Abort() error {
	errc := t.WriteCloser.Close()
	errd := t.c.ObjectDelete(t.container, t.name)
	if errc != nil {
		return errc
	}
	if errd != nil {
		return errd
	}
	return nil
}

func (t *thumb) Commit() error {
	return t.WriteCloser.Close()
}

func (t *thumbsV2) CreateThumb(img *vfs.FileDoc, format string) (vfs.ThumbFiler, error) {
	name := t.makeName(img, format)
	objMeta := swift.Metadata{
		"file-md5": hex.EncodeToString(img.MD5Sum),
	}
	obj, err := t.c.ObjectCreate(t.container, name, true, "", img.Mime,
		objMeta.ObjectHeaders())
	if err != nil {
		if _, _, errc := t.c.Container(t.container); errc == swift.ContainerNotFound {
			if errc = t.c.ContainerCreate(t.container, nil); errc != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	th := &thumb{
		WriteCloser: obj,
		c:           t.c,
		container:   t.container,
		name:        name,
	}
	return th, nil
}

func (t *thumbsV2) ThumbExists(img *vfs.FileDoc, format string) (bool, error) {
	name := t.makeName(img, format)
	infos, headers, err := t.c.Object(t.container, name)
	if err == swift.ObjectNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if infos.Bytes == 0 {
		return false, nil
	}
	if md5 := headers["file-md5"]; md5 != "" {
		var md5sum []byte
		md5sum, err = hex.DecodeString(md5)
		if err == nil && !bytes.Equal(md5sum, img.MD5Sum) {
			return false, nil
		}
	}
	return true, nil
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
	return fmt.Sprintf("thumbs/%s-%s", MakeObjectName(img.ID()), format)
}
