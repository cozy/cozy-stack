package thumbnail

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/note"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/realtime"
	multierror "github.com/hashicorp/go-multierror"
)

type ImageMessage struct {
	NoteImage *note.Image `json:"noteImage,omitempty"`
	// -- or --
	File   *vfs.FileDoc `json:"file,omitempty"`
	Format string       `json:"format,omitempty"`
}

type imageEvent struct {
	Verb   string       `json:"verb"`
	Doc    vfs.FileDoc  `json:"doc"`
	OldDoc *vfs.FileDoc `json:"old,omitempty"`
}

var formats = map[string]string{
	"tiny":   "96x96",
	"small":  "640x480>",
	"medium": "1280x720>",
	"large":  "1920x1080>",
	"note":   "768x",
}

func init() {
	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "thumbnail",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 2,
		Reserved:     true,
		Timeout:      30 * time.Second,
		WorkerFunc:   Worker,
	})

	job.AddWorker(&job.WorkerConfig{
		WorkerType:   "thumbnailck",
		Concurrency:  runtime.NumCPU(),
		MaxExecCount: 1,
		Reserved:     true,
		Timeout:      24 * time.Hour,
		WorkerFunc:   WorkerCheck,
	})
}

// Worker is a worker that creates thumbnails for photos and images.
func Worker(ctx *job.WorkerContext) error {
	var msg ImageMessage
	if err := ctx.UnmarshalMessage(&msg); err != nil {
		return err
	}
	log := ctx.Logger()

	if msg.NoteImage != nil {
		return resizeNoteImage(ctx, msg.NoteImage)
	}
	if msg.File != nil {
		mutex := config.Lock().ReadWrite(ctx.Instance, "thumbnails/"+msg.File.ID())
		if err := mutex.Lock(); err != nil {
			return err
		}
		defer mutex.Unlock()
		log.Debugf("%s %s", msg.File.ID(), msg.Format)
		if _, ok := formats[msg.Format]; !ok {
			return errors.New("invalid format")
		}
		return generateSingleThumbnail(ctx, msg.File, msg.Format)
	}

	var img imageEvent
	if err := ctx.UnmarshalEvent(&img); err != nil {
		return err
	}
	if img.Verb != "DELETED" && img.Doc.Trashed {
		return nil
	}
	if img.OldDoc != nil && sameImg(&img.Doc, img.OldDoc) {
		return nil
	}

	mutex := config.Lock().ReadWrite(ctx.Instance, "thumbnails/"+img.Doc.ID())
	if err := mutex.Lock(); err != nil {
		return err
	}
	defer mutex.Unlock()
	log.Debugf("%s %s", img.Verb, img.Doc.ID())

	switch img.Verb {
	case "CREATED":
		return generateThumbnails(ctx, &img.Doc)
	case "UPDATED":
		if err := removeThumbnails(ctx.Instance, &img.Doc); err != nil {
			log.Debugf("failed to remove thumbnails for %s: %s", img.Doc.ID(), err)
		}
		return generateThumbnails(ctx, &img.Doc)
	case "DELETED":
		return removeThumbnails(ctx.Instance, &img.Doc)
	}
	return fmt.Errorf("unknown type %s for event", img.Verb)
}

func sameImg(doc, old *vfs.FileDoc) bool {
	// XXX It is needed for a file that has just been uploaded. The first
	// revision will have the size and md5sum, but is marked as trashed,
	// and we have to wait for the second revision to have the file to generate
	// the thumbnails
	if doc.Trashed != old.Trashed {
		return false
	}
	if doc.ByteSize != old.ByteSize {
		return false
	}
	return bytes.Equal(doc.MD5Sum, old.MD5Sum)
}

type thumbnailMsg struct {
	WithMetadata bool `json:"with_metadata"`
}

// WorkerCheck is a worker function that checks all the images to generate
// missing thumbnails.
func WorkerCheck(ctx *job.WorkerContext) error {
	var msg thumbnailMsg
	if err := ctx.UnmarshalMessage(&msg); err != nil {
		return err
	}
	fs := ctx.Instance.VFS()
	fsThumb := ctx.Instance.ThumbsFS()
	var errm error
	_ = vfs.Walk(fs, "/", func(name string, dir *vfs.DirDoc, img *vfs.FileDoc, err error) error {
		if err != nil {
			return err
		}
		if dir != nil || img.Class != "image" {
			return nil
		}
		allExists := true
		for _, format := range vfs.ThumbnailFormatNames {
			var exists bool
			exists, err = fsThumb.ThumbExists(img, format)
			if err != nil {
				errm = multierror.Append(errm, err)
				return nil
			}
			if !exists {
				allExists = false
			}
		}
		if !allExists {
			if err = generateThumbnails(ctx, img); err != nil {
				errm = multierror.Append(errm, err)
			}
		}
		if msg.WithMetadata {
			var meta *vfs.Metadata
			meta, err = calculateMetadata(fs, img)
			if err != nil {
				errm = multierror.Append(errm, err)
			}
			if meta != nil {
				newImg := img.Clone().(*vfs.FileDoc)
				newImg.Metadata = *meta
				if newImg.CozyMetadata == nil {
					newImg.CozyMetadata = vfs.NewCozyMetadata(ctx.Instance.PageURL("/", nil))
				} else {
					newImg.CozyMetadata.UpdatedAt = time.Now()
				}
				if err = fs.UpdateFileDoc(img, newImg); err != nil {
					errm = multierror.Append(errm, err)
				}
			}
		}
		return nil
	})
	return errm
}

func calculateMetadata(fs vfs.VFS, img *vfs.FileDoc) (*vfs.Metadata, error) {
	exifP := vfs.NewMetaExtractor(img)
	if exifP == nil {
		return nil, nil
	}
	exif := *exifP
	f, err := fs.OpenFile(img)
	if err != nil {
		return nil, err
	}
	defer func() {
		if errc := f.Close(); err == nil {
			err = errc
		}
	}()
	_, err = io.Copy(exif, io.LimitReader(f, 128*1024))
	if err != nil {
		return nil, err
	}
	meta := exif.Result()
	return &meta, nil
}

func generateSingleThumbnail(ctx *job.WorkerContext, img *vfs.FileDoc, format string) error {
	if ok := checkByteSize(img); !ok {
		return nil
	}

	fs := ctx.Instance.ThumbsFS()
	exists, err := fs.ThumbExists(img, format)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	var in io.Reader
	in, err = ctx.Instance.VFS().OpenFile(img)
	if err != nil {
		return err
	}

	var env []string
	{
		var tempDir string
		tempDir, err = os.MkdirTemp("", "magick")
		if err == nil {
			defer os.RemoveAll(tempDir)
			envTempDir := fmt.Sprintf("MAGICK_TEMPORARY_PATH=%s", tempDir)
			env = []string{envTempDir}
		}
	}
	_, err = recGenerateThumb(ctx, in, fs, img, format, env, true)
	return err
}

func generateThumbnails(ctx *job.WorkerContext, img *vfs.FileDoc) error {
	if ok := checkByteSize(img); !ok {
		return nil
	}

	fs := ctx.Instance.ThumbsFS()
	var in io.Reader
	in, err := ctx.Instance.VFS().OpenFile(img)
	if err != nil {
		return err
	}

	var env []string
	{
		var tempDir string
		tempDir, err = os.MkdirTemp("", "magick")
		if err == nil {
			defer os.RemoveAll(tempDir)
			envTempDir := fmt.Sprintf("MAGICK_TEMPORARY_PATH=%s", tempDir)
			env = []string{envTempDir}
		}
	}

	if img.Class == "image" {
		in, err = recGenerateThumb(ctx, in, fs, img, "large", env, false)
		if err != nil {
			return err
		}
		in, err = recGenerateThumb(ctx, in, fs, img, "medium", env, false)
		if err != nil {
			return err
		}
		in, err = recGenerateThumb(ctx, in, fs, img, "small", env, false)
		if err != nil {
			return err
		}
	}

	exists, err := fs.ThumbExists(img, "tiny")
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = recGenerateThumb(ctx, in, fs, img, "tiny", env, true)
	return err
}

func checkByteSize(img *vfs.FileDoc) bool {
	// Do not try to generate thumbnails for images that weight more than 100MB
	// (or 5MB for PSDs)
	var limit int64 = 100 * 1024 * 1024
	if img.Mime == "image/vnd.adobe.photoshop" {
		limit = 5 * 1024 * 1024
	}
	return img.ByteSize < limit
}

func recGenerateThumb(ctx *job.WorkerContext, in io.Reader, fs vfs.Thumbser, img *vfs.FileDoc, format string, env []string, noOuput bool) (r io.Reader, err error) {
	defer func() {
		if inCloser, ok := in.(io.Closer); ok {
			if errc := inCloser.Close(); errc != nil && err == nil {
				err = errc
			}
		}
	}()
	th, err := fs.CreateThumb(img, format)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = th.Abort()
		} else {
			_ = th.Commit()
			doc := &couchdb.JSONDoc{
				M: map[string]interface{}{
					"_id":    img.ID(),
					"format": format,
				},
				Type: consts.Thumbnails,
			}
			go realtime.GetHub().Publish(ctx.Instance, realtime.EventCreate, doc, nil)
		}
	}()
	var buffer *bytes.Buffer
	var out io.Writer
	if noOuput {
		out = th
	} else {
		buffer = new(bytes.Buffer)
		out = io.MultiWriter(th, buffer)
	}
	err = generateThumb(ctx, in, out, img.ID(), format, env)
	if err != nil {
		return nil, err
	}
	return buffer, nil
}

// The thumbnails are generated with ImageMagick, because it has the better
// compromise for speed, quality and ease of deployment.
// See https://github.com/fawick/speedtest-resize
//
// We are using some complicated ImageMagick options to optimize the speed and
// quality of the generated thumbnails.
// See https://www.smashingmagazine.com/2015/06/efficient-image-resizing-with-imagemagick/
func generateThumb(ctx *job.WorkerContext, in io.Reader, out io.Writer, fileID string, format string, env []string) error {
	convertCmd := config.GetConfig().Jobs.ImageMagickConvertCmd
	if convertCmd == "" {
		convertCmd = "convert"
	}
	quality := "82" // A good compromise between file size and quality
	if format == "tiny" {
		quality = "99" // At small resolution, we want a very good quality
	}
	args := []string{
		"-limit", "Memory", "2GB",
		"-limit", "Map", "3GB",
		"-[0]",         // Takes the input from stdin
		"-auto-orient", // Rotate image according to the EXIF metadata
		"-strip",       // Strip the EXIF metadata
		"-quality", quality,
		"-interlace", "none", // Don't use progressive JPEGs, they are heavier
		"-thumbnail", formats[format], // Makes a thumbnail that fits inside the given format
		"-background", "white", // Use white for the background
		"-alpha", "remove", // JPEGs don't have an alpha channel
		"-colorspace", "sRGB", // Use the colorspace recommended for web, sRGB
		"jpg:-", // Send the output on stdout, in JPEG format
	}
	var stderr bytes.Buffer
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctxWithTimeout, convertCmd, args...)
	cmd.Env = env
	cmd.Stdin = in
	cmd.Stdout = out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Truncate very long messages
		msg := stderr.String()
		if len(msg) > 4000 {
			msg = msg[:4000]
		}
		ctx.Logger().
			WithField("stderr", msg).
			WithField("file_id", fileID).
			Errorf("imagemagick failed: %s", err)
		return err
	}
	return nil
}

func removeThumbnails(i *instance.Instance, img *vfs.FileDoc) error {
	return i.ThumbsFS().RemoveThumbs(img, vfs.ThumbnailFormatNames)
}

func resizeNoteImage(ctx *job.WorkerContext, img *note.Image) error {
	fs := ctx.Instance.ThumbsFS()
	in, err := fs.OpenNoteThumb(img.ID(), consts.NoteImageOriginalFormat)
	if err != nil {
		return err
	}
	defer func() {
		if errc := in.Close(); errc != nil && err == nil {
			err = errc
		}
	}()

	var env []string
	{
		tempDir, err := os.MkdirTemp("", "magick")
		if err == nil {
			defer os.RemoveAll(tempDir)
			envTempDir := fmt.Sprintf("MAGICK_TEMPORARY_PATH=%s", tempDir)
			env = []string{envTempDir}
		}
	}

	var th vfs.ThumbFiler
	th, err = fs.CreateNoteThumb(img.ID(), "image/jpeg", consts.NoteImageThumbFormat)
	if err != nil {
		return err
	}

	out := th
	if err = generateThumb(ctx, in, out, img.ID(), "note", env); err != nil {
		return err
	}

	if err = th.Commit(); err != nil {
		return err
	}

	img.ToResize = false
	_ = couchdb.UpdateDoc(ctx.Instance, img)

	event := note.Event{
		"width":   note.MaxWidth,
		"height":  img.Height * note.MaxWidth / img.Width,
		"mime":    "image/jpeg",
		"doctype": consts.NotesImages,
	}
	event.SetID(img.ID())
	note.PublishThumbnail(ctx.Instance, event)
	return nil
}
