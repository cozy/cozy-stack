package thumbnail

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/vfs"
)

type imageEvent struct {
	Verb string      `json:"Verb"`
	Doc  vfs.FileDoc `json:"Doc"`
}

var formats = map[string]string{
	"small":  "640x480",
	"medium": "1280x720",
	"large":  "1920x1080",
}

func init() {
	jobs.AddWorker("thumbnail", &jobs.WorkerConfig{
		Concurrency:  (runtime.NumCPU() + 1) / 2,
		MaxExecCount: 2,
		Timeout:      15 * time.Second,
		WorkerFunc:   Worker,
	})
}

// Worker is a worker that creates thumbnails for photos and images.
func Worker(ctx context.Context, m jobs.Message) error {
	domain := ctx.Value(jobs.ContextDomainKey).(string)
	e, ok := ctx.Value(jobs.ContextEventKey).(jobs.Event)
	if !ok {
		return errors.New("Missing realtime event from context")
	}

	var img imageEvent
	if err := e.Unmarshal(&img); err != nil {
		return err
	}
	if img.Verb != "DELETED" && img.Doc.Trashed {
		return nil
	}

	log := logger.WithDomain(domain)
	log.Infof("[jobs] thumbnail: %s %s", img.Verb, img.Doc.ID())
	i, err := instance.Get(domain)
	if err != nil {
		return err
	}
	switch img.Verb {
	case "CREATED":
		return generateThumbnails(ctx, i, &img.Doc)
	case "UPDATED":
		if err = removeThumbnails(i, &img.Doc); err != nil {
			log.Infof("[jobs] failed to remove thumbnails for %s", img.Doc.ID())
		}
		return generateThumbnails(ctx, i, &img.Doc)
	case "DELETED":
		return removeThumbnails(i, &img.Doc)
	}
	return fmt.Errorf("Unknown type %s for image event", img.Verb)
}

func generateThumbnails(ctx context.Context, i *instance.Instance, img *vfs.FileDoc) error {
	fs := i.ThumbsFS()
	var in io.Reader
	in, err := i.VFS().OpenFile(img)
	if err != nil {
		return err
	}
	in, err = recGenerateThub(ctx, in, fs, img, "large")
	if err != nil {
		return err
	}
	in, err = recGenerateThub(ctx, in, fs, img, "medium")
	if err != nil {
		return err
	}
	// TODO(optim): no need for the last output
	_, err = recGenerateThub(ctx, in, fs, img, "small")
	return err
}

func recGenerateThub(ctx context.Context, in io.Reader, fs vfs.Thumbser, img *vfs.FileDoc, format string) (r io.Reader, err error) {
	defer func() {
		if inCloser, ok := in.(io.Closer); ok {
			if errc := inCloser.Close(); errc != nil && err == nil {
				err = errc
			}
		}
	}()
	file, err := fs.CreateThumb(img, format)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	buffer := new(bytes.Buffer)
	ws := io.MultiWriter(file, buffer)
	err = generateThumb(ctx, in, ws, format)
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
func generateThumb(ctx context.Context, in io.Reader, out io.Writer, format string) error {
	args := []string{
		"-limit", "Memory", "2GB",
		"-limit", "Map", "3GB",
		"-",              // Takes the input from stdin
		"-auto-orient",   // Rotate image according to the EXIF metadata
		"-strip",         // Strip the EXIF metadata
		"-quality", "82", // A good compromise between file size and quality
		"-interlace", "none", // Don't use progressive JPEGs, they are heavier
		"-thumbnail", formats[format], // Makes a thumbnail that fits inside the given format
		"-colorspace", "sRGB", // Use the colorspace recommended for web, sRGB
		"jpg:-", // Send the output on stdout, in JPEG format
	}
	cmd := exec.CommandContext(ctx, "convert", args...) // #nosec
	cmd.Stdin = in
	cmd.Stdout = out
	return cmd.Run()
}

func removeThumbnails(i *instance.Instance, img *vfs.FileDoc) error {
	var e error
	for format := range formats {
		if err := i.ThumbsFS().RemoveThumb(img, format); err != nil {
			e = err
		}
	}
	return e
}
