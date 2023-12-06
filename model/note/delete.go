package note

import (
	"fmt"
	"runtime"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

func init() {
	vfs.DeleteNote = deleteNote
}

func deleteNote(db prefixer.Prefixer, noteID string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				var err error
				switch r := r.(type) {
				case error:
					err = r
				default:
					err = fmt.Errorf("%v", r)
				}
				stack := make([]byte, 4<<10) // 4 KB
				length := runtime.Stack(stack, false)
				log := logger.WithNamespace("note").WithField("panic", true)
				log.Errorf("PANIC RECOVER %s: %s", err.Error(), stack[:length])
			}
		}()

		images, err := getImages(db, noteID)
		if err == nil && len(images) > 0 {
			formats := []string{
				consts.NoteImageOriginalFormat,
				consts.NoteImageThumbFormat,
			}
			inst, err := instance.Get(db.DomainName())
			if err == nil {
				for _, img := range images {
					_ = inst.ThumbsFS().RemoveNoteThumb(img.ID(), formats)
					_ = couchdb.DeleteDoc(db, img)
				}
			}
		}

		steps, err := getSteps(db, noteID, 0)
		if err == nil && len(steps) > 0 {
			docs := make([]couchdb.Doc, 0, len(steps))
			for i := range steps {
				docs = append(docs, &steps[i])
			}
			_ = couchdb.BulkDeleteDocs(db, consts.NotesSteps, docs)
		}
	}()
}
