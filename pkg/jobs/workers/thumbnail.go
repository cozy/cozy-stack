package workers

import (
	"context"
	"fmt"
	"time"

	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/vfs"
)

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
	domain := ctx.Value(jobs.ContextDomainKey).(string)
	msg := &imageMessage{}
	if err := m.Unmarshal(msg); err != nil {
		return err
	}
	fmt.Printf("[jobs] thumbnails %s: %#v", domain, msg)
	switch msg.Event.Type {
	case "CREATED":
		return generateThumbnails(domain, msg.Event.Doc)
	case "UPDATED":
		if err := removeThumbnails(domain, msg.Event.Doc); err != nil {
			return err
		}
		return generateThumbnails(domain, msg.Event.Doc)
	case "DELETED":
		return removeThumbnails(domain, msg.Event.Doc)
	}
	return fmt.Errorf("Unknown type %s for image event", msg.Event.Type)
}

func generateThumbnails(domain string, image vfs.FileDoc) error {
	return nil
}

func removeThumbnails(domain string, image vfs.FileDoc) error {
	return nil
}
