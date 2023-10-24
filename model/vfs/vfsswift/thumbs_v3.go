package vfsswift

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/labstack/echo/v4"
	"github.com/ncw/swift/v2"
)

var unixEpochZero = time.Time{}

// NewThumbsFsV3 creates a new thumb filesystem base on swift.
//
// This version stores the thumbnails in the same container as the main data
// container.
func NewThumbsFsV3(c *swift.Connection, db prefixer.Prefixer) vfs.Thumbser {
	return &thumbsV3{
		c:         c,
		container: swiftV3ContainerPrefix + db.DBPrefix(),
		ctx:       context.Background(),
	}
}

type thumbsV3 struct {
	c         *swift.Connection
	container string
	ctx       context.Context
}

type thumb struct {
	io.WriteCloser
	c         *swift.Connection
	container string
	name      string
}

func (t *thumb) Abort() error {
	ctx := context.Background()
	errc := t.WriteCloser.Close()
	errd := t.c.ObjectDelete(ctx, t.container, t.name)
	// Create an empty file that indicates that the thumbnail generation has failed
	_ = t.c.ObjectPutString(ctx, t.container, t.name, "", echo.MIMEOctetStream)
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

func (t *thumbsV3) CreateThumb(img *vfs.FileDoc, format string) (vfs.ThumbFiler, error) {
	name := t.makeName(img.ID(), format)
	objMeta := swift.Metadata{
		"file-md5": hex.EncodeToString(img.MD5Sum),
	}
	obj, err := t.c.ObjectCreate(t.ctx, t.container, name, true, "", "image/jpeg", objMeta.ObjectHeaders())
	if err != nil {
		if _, _, errc := t.c.Container(t.ctx, t.container); errc == swift.ContainerNotFound {
			if errc = t.c.ContainerCreate(t.ctx, t.container, nil); errc != nil {
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

func (t *thumbsV3) ThumbExists(img *vfs.FileDoc, format string) (bool, error) {
	name := t.makeName(img.ID(), format)
	_, headers, err := t.c.Object(t.ctx, t.container, name)
	if errors.Is(err, swift.ObjectNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	meta := headers.ObjectMetadata()
	if md5 := meta["file-md5"]; md5 != "" {
		var md5sum []byte
		md5sum, err = hex.DecodeString(md5)
		if err == nil && !bytes.Equal(md5sum, img.MD5Sum) {
			return false, nil
		}
	}
	return true, nil
}

func (t *thumbsV3) RemoveThumbs(img *vfs.FileDoc, formats []string) error {
	objNames := make([]string, len(formats))
	for i, format := range formats {
		objNames[i] = t.makeName(img.ID(), format)
	}
	_, err := t.c.BulkDelete(t.ctx, t.container, objNames)
	return err
}

func (t *thumbsV3) ServeThumbContent(w http.ResponseWriter, req *http.Request, img *vfs.FileDoc, format string) error {
	name := t.makeName(img.ID(), format)
	f, o, err := t.c.ObjectOpen(t.ctx, t.container, name, false, nil)
	if err != nil {
		return wrapSwiftErr(err)
	}
	defer f.Close()

	ctype := o["Content-Type"]
	if ctype == echo.MIMEOctetStream {
		// We have some old images where the thumbnail has not been correctly
		// saved in Swift. We should delete the thumbnail to allow another try.
		if l, err := f.Length(t.ctx); err == nil && l > 0 {
			_ = t.RemoveThumbs(img, vfs.ThumbnailFormatNames)
			return os.ErrNotExist
		}
		// Image magick has failed to generate a thumbnail, and retrying would
		// be useless.
		return os.ErrInvalid
	}

	w.Header().Set("Etag", fmt.Sprintf(`"%s"`, o["Etag"]))
	w.Header().Set("Content-Type", ctype)
	http.ServeContent(w, req, name, unixEpochZero, &backgroundSeeker{f})
	return nil
}

func (t *thumbsV3) CreateNoteThumb(id, mime, format string) (vfs.ThumbFiler, error) {
	name := t.makeName(id, format)
	obj, err := t.c.ObjectCreate(t.ctx, t.container, name, true, "", mime, nil)
	if err != nil {
		if _, _, errc := t.c.Container(t.ctx, t.container); errc == swift.ContainerNotFound {
			if errc = t.c.ContainerCreate(t.ctx, t.container, nil); errc != nil {
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

func (t *thumbsV3) OpenNoteThumb(id, format string) (io.ReadCloser, error) {
	name := t.makeName(id, format)
	obj, _, err := t.c.ObjectOpen(t.ctx, t.container, name, false, nil)
	if errors.Is(err, swift.ObjectNotFound) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func (t *thumbsV3) RemoveNoteThumb(id string, formats []string) error {
	objNames := make([]string, len(formats))
	for i, format := range formats {
		objNames[i] = t.makeName(id, format)
	}
	_, err := t.c.BulkDelete(t.ctx, t.container, objNames)
	if err != nil {
		logger.WithNamespace("vfsswift").Infof("Cannot remove note thumbs: %s", err)
	}
	return err
}

func (t *thumbsV3) ServeNoteThumbContent(w http.ResponseWriter, req *http.Request, id string) error {
	name := t.makeName(id, consts.NoteImageThumbFormat)
	f, o, err := t.c.ObjectOpen(t.ctx, t.container, name, false, nil)
	if err != nil {
		name = t.makeName(id, consts.NoteImageOriginalFormat)
		f, o, err = t.c.ObjectOpen(t.ctx, t.container, name, false, nil)
		if err != nil {
			return wrapSwiftErr(err)
		}
	}
	defer f.Close()

	w.Header().Set("Etag", fmt.Sprintf(`"%s"`, o["Etag"]))
	w.Header().Set("Content-Type", o["Content-Type"])
	http.ServeContent(w, req, name, unixEpochZero, &backgroundSeeker{f})
	return nil
}

func (t *thumbsV3) makeName(imgID string, format string) string {
	return fmt.Sprintf("thumbs/%s-%s", MakeObjectName(imgID), format)
}

// MakeObjectName build the swift object name for a given file document.It
// creates a virtual subfolder by splitting the document ID, which should be 32
// bytes long, on the 27nth byte. This avoid having a flat hierarchy in swift
// with no bound
func MakeObjectName(docID string) string {
	if len(docID) != 32 {
		return docID
	}
	return docID[:22] + "/" + docID[22:27] + "/" + docID[27:]
}

func makeDocID(objName string) string {
	if len(objName) != 34 {
		return objName
	}
	return objName[:22] + objName[23:28] + objName[29:]
}

func wrapSwiftErr(err error) error {
	if errors.Is(err, swift.ObjectNotFound) || errors.Is(err, swift.ContainerNotFound) {
		return os.ErrNotExist
	}
	return err
}

type backgroundSeeker struct {
	*swift.ObjectOpenFile
}

func (f *backgroundSeeker) Seek(offset int64, whence int) (int64, error) {
	return f.ObjectOpenFile.Seek(context.Background(), offset, whence)
}
