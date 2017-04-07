package workers

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/vfs"
)

var formats = map[string]string{
	"small":  "640x480",
	"medium": "1280x720",
	"large":  "1920x1080",
}

type imageMessage struct {
	Event struct {
		Type string      `json:"Type"`
		Doc  vfs.FileDoc `json:"Doc"`
	} `json:"event"`
}

func init() {
	jobs.AddWorker("thumbnail", &jobs.WorkerConfig{
		Concurrency:  4,
		MaxExecCount: 3,
		Timeout:      10 * time.Second,
		WorkerFunc:   ThumbnailWorker,
	})
}

// ThumbnailWorker is a worker that creates thumbnails for photos and images.
func ThumbnailWorker(ctx context.Context, m *jobs.Message) error {
	msg := &imageMessage{}
	if err := m.Unmarshal(msg); err != nil {
		return err
	}
	domain := ctx.Value(jobs.ContextDomainKey).(string)
	log.Debugf("[jobs] thumbnail %s: %#v", domain, msg.Event)
	fmt.Printf("[jobs] thumbnail %s: %#v", domain, msg.Event.Doc)
	i, err := instance.Get(domain)
	if err != nil {
		return err
	}
	switch msg.Event.Type {
	case "CREATED":
		return generateThumbnails(ctx, i, &msg.Event.Doc)
	case "UPDATED":
		if err = removeThumbnails(i, &msg.Event.Doc); err != nil {
			return err
		}
		return generateThumbnails(ctx, i, &msg.Event.Doc)
	case "DELETED":
		return removeThumbnails(i, &msg.Event.Doc)
	}
	return fmt.Errorf("Unknown type %s for image event", msg.Event.Type)
}

func generateThumbnails(ctx context.Context, i *instance.Instance, img *vfs.FileDoc) error {
	fs := i.ThumbsFS()
	if err := fs.MkdirAll(vfs.ThumbDir(img), 0755); err != nil {
		return err
	}
	flags := os.O_RDWR | os.O_CREATE
	in, err := i.VFS().OpenFile(img)
	if err != nil {
		return err
	}
	defer in.Close()
	largeName := vfs.ThumbPath(img, "large")
	large, err := fs.OpenFile(largeName, flags, 0640)
	if err != nil {
		return err
	}
	defer large.Close()
	if err = generateThumb(ctx, in, large, formats["large"]); err != nil {
		return err
	}
	mediumName := vfs.ThumbPath(img, "medium")
	medium, err := fs.OpenFile(mediumName, flags, 0640)
	if err != nil {
		return err
	}
	defer medium.Close()
	if _, err = large.Seek(0, 0); err != nil {
		return err
	}
	if err = generateThumb(ctx, large, medium, formats["medium"]); err != nil {
		return err
	}
	smallName := vfs.ThumbPath(img, "small")
	small, err := fs.OpenFile(smallName, flags, 0640)
	if err != nil {
		return err
	}
	defer small.Close()
	if _, err = medium.Seek(0, 0); err != nil {
		return err
	}
	return generateThumb(ctx, medium, small, formats["small"])
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
		"-strip",         // Strip the EXIF metadata
		"-quality", "82", // A good compromise between file size and quality
		"-interlace", "none", // Don't use progressive JPEGs, they are heavier
		"-thumbnail", format, // Makes a thumbnail that fits inside the given format
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
		filepath := vfs.ThumbPath(img, format)
		if err := i.ThumbsFS().Remove(filepath); err != nil {
			e = err
		}
	}
	return e
}
