package note

import (
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

func init() {
	vfs.DeleteNote = deleteNote
}

func deleteNote(db prefixer.Prefixer, noteID string) {
	go func() {
		images, err := getImages(db, noteID)
		if err == nil {
			for _, img := range images {
				inst := &instance.Instance{
					Domain: db.DomainName(),
					Prefix: db.DBPrefix(),
				}
				_ = inst.ThumbsFS().RemoveNoteThumb(img.ID())
				_ = couchdb.DeleteDoc(db, img)
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
